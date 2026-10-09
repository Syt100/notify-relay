package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Syt100/notify-relay/internal/config"
	"github.com/Syt100/notify-relay/internal/message"
	"github.com/Syt100/notify-relay/internal/provider"
	"github.com/Syt100/notify-relay/internal/queue"
)

func TestTruncatedReplayIsVisible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Messages-Truncated", "1")
		_, _ = fmt.Fprintln(w, `{"event":"open","time":100,"topic":"notify"}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := testConfig()
	c.BaseURL = srv.URL
	s := makeService(t, c, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.subscribe(ctx, "notify") }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/readyz", nil))
		if strings.Contains(rr.Body.String(), `"replay_truncated":true`) {
			if rr.Code != 503 {
				t.Fatal("truncated stream marked ready")
			}
			cancel()
			<-done
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("truncated replay not surfaced")
}

type cooldownSender struct {
	calls atomic.Int32
	first chan struct{}
}

func (p *cooldownSender) Send(context.Context, message.Message) error {
	if p.calls.Add(1) == 1 {
		close(p.first)
		return &provider.DeliveryError{Reason: "provider HTTP 429", After: 2 * time.Second}
	}
	return nil
}
func TestRetryAfterPacesOtherMessages(t *testing.T) {
	p := &cooldownSender{first: make(chan struct{})}
	c := testConfig()
	c.RetryBase = 3 * time.Second
	c.RetryMax = 3 * time.Second
	s := makeService(t, c, p)
	for _, id := range []string{"a", "b"} {
		_, e := s.store.Enqueue(message.Message{ID: id, Topic: "notify", Time: 100, Event: "message"})
		if e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.worker(ctx) }()
	<-p.first
	time.Sleep(1200 * time.Millisecond)
	// The longer per-message retry does not prevent the separate pending message
	// from honoring the global Retry-After cooldown.
	if p.calls.Load() != 1 {
		t.Fatal("global pace violated")
	}
	cancel()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func testConfig() config.Config {
	return config.Config{Token: "tk_test", Topics: []string{"notify"}, Stale: time.Second, RetryBase: time.Millisecond * 10, RetryMax: time.Millisecond * 100, Retention: time.Hour, MinPriority: 1}
}
func makeService(t *testing.T, c config.Config, p Sender) *Service {
	t.Helper()
	q, e := queue.Open(filepath.Join(t.TempDir(), "state.bolt"), 100)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = q.Close() })
	return New(c, q, p, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
func TestReconnectQuietCheckpointAndDedup(t *testing.T) {
	var calls atomic.Int32
	var s *Service
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tk_test" {
			t.Error("authorization")
		}
		n := calls.Add(1)
		if n == 1 {
			if r.URL.Query().Get("since") != "" {
				t.Error("first replay")
			}
			_, _ = fmt.Fprintln(w, `{"event":"open","time":100,"topic":"notify"}`)
			return
		}
		if r.URL.Query().Get("since") != "99" {
			t.Error("quiet stream checkpoint lost", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintln(w, `{"event":"open","time":105,"topic":"notify"}`)
		for i := 0; i < 2; i++ {
			_, _ = fmt.Fprintln(w, `{"event":"message","id":"a","time":101,"topic":"notify","message":"hello"}`)
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := testConfig()
	c.BaseURL = srv.URL
	s = makeService(t, c, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e := s.subscribe(ctx, "notify"); e == nil {
		t.Fatal("EOF expected")
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- s.subscribe(ctx, "notify") }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n, _, e := s.store.Counts()
		if e != nil {
			t.Fatal(e)
		}
		if n == 1 {
			cancel()
			<-secondDone
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("not enqueued")
}
func TestStaleStreamCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"event":"open","time":100,"topic":"notify"}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := testConfig()
	c.BaseURL = srv.URL
	c.Stale = 30 * time.Millisecond
	s := makeService(t, c, nil)
	start := time.Now()
	if e := s.subscribe(context.Background(), "notify"); e == nil {
		t.Fatal("stale error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("hung stream")
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/readyz", nil))
	if rr.Code != 503 {
		t.Fatal("stale stream is ready")
	}
}
func TestEndToEndRetry(t *testing.T) {
	var sends atomic.Int32
	accepted := make(chan struct{}, 1)
	wx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p message.Push
		if e := json.NewDecoder(r.Body).Decode(&p); e != nil {
			t.Error(e)
		}
		if p.Summary != "🚨 test" || p.URL != "https://example.com" {
			t.Error("mapping")
		}
		if sends.Add(1) == 1 {
			_, _ = fmt.Fprint(w, `{"code":1001,"msg":"SPT_secret"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":1000,"success":true}`)
		accepted <- struct{}{}
	}))
	defer wx.Close()
	nt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{"event":"open","time":100,"topic":"notify"}`)
		_, _ = fmt.Fprintln(w, `{"event":"message","id":"a","time":101,"topic":"notify","title":"test","priority":5,"message":"hi","click":"https://example.com"}`)
		w.(http.Flusher).Flush()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = fmt.Fprintln(w, `{"event":"keepalive","time":102,"topic":"notify"}`)
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer nt.Close()
	c := testConfig()
	c.BaseURL = nt.URL
	s := makeService(t, c, provider.New(wx.URL, "SPT_secret", time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-accepted:
	case <-time.After(4 * time.Second):
		t.Fatal("no successful retry")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n, d, e := s.store.Counts()
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 && d == 1 {
			rr := httptest.NewRecorder()
			s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/readyz", nil))
			if rr.Code != 200 || strings.Contains(rr.Body.String(), "SPT_secret") {
				t.Fatal(rr.Body.String())
			}
			cancel()
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			if sends.Load() != 2 {
				t.Fatal("send count")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("not acknowledged")
}

func TestMultiTopicAndPriorityFilter(t *testing.T) {
	var sends atomic.Int32
	wx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sends.Add(1); _, _ = fmt.Fprint(w, `{"code":1000}`) }))
	defer wx.Close()
	nt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		topic := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		priority := 2
		if topic == "high" {
			priority = 4
		}
		_, _ = fmt.Fprintf(w, "{\"event\":\"open\",\"time\":100,\"topic\":%q}\n", topic)
		_, _ = fmt.Fprintf(w, "{\"event\":\"message\",\"id\":\"same\",\"time\":101,\"topic\":%q,\"priority\":%d}\n", topic, priority)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer nt.Close()
	c := testConfig()
	c.Topics = []string{"low", "high"}
	c.MinPriority = 4
	c.BaseURL = nt.URL
	s := makeService(t, c, provider.New(wx.URL, "SPT_test", time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n, d, e := s.store.Counts()
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 && d == 2 {
			cancel()
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			if sends.Load() != 1 {
				t.Fatal("filter failed")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("multi-topic flow incomplete")
}
