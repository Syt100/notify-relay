package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Syt100/notify-relay/internal/message"
	"github.com/Syt100/notify-relay/internal/provider"
)

func TestRejectedMessageIsIsolatedWithoutBlockingNext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p message.Push
		json.NewDecoder(r.Body).Decode(&p)
		if p.Summary == "[notify] bad" {
			w.WriteHeader(422)
			return
		}
		fmt.Fprint(w, `{"code":1000}`)
	}))
	defer srv.Close()
	s := makeService(t, testConfig(), provider.New(srv.URL, "test", time.Second))
	for _, id := range []string{"bad", "good"} {
		if _, err := s.store.Enqueue(message.Message{ID: id, Topic: "notify", Title: id, Time: 100}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.worker(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		p, d, err := s.store.Counts()
		if err != nil {
			t.Fatal(err)
		}
		if p == 0 && d == 1 {
			rr := httptest.NewRecorder()
			s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
			var state map[string]any
			json.Unmarshal(rr.Body.Bytes(), &state)
			if state["failed"] != float64(1) {
				t.Fatal("isolated failure is not visible", state)
			}
			return
		}
		time.Sleep(time.Millisecond * 10)
	}
	t.Fatal("permanent failure must leave pending queue; next message must be accepted")
}

func TestIsolationKeepsGlobalCooldown(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(422)
	}))
	defer srv.Close()
	s := makeService(t, testConfig(), provider.New(srv.URL, "test", time.Second))
	for _, id := range []string{"a", "b"} {
		if _, err := s.store.Enqueue(message.Message{ID: id, Topic: "notify", Time: 100}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.worker(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("no provider call")
	}
	time.Sleep(1200 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatal("isolation bypassed provider cooldown")
	}
}

func TestBusinessRejectionIsIsolatedAfterFiveAttempts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"code":1001,"msg":"private provider detail"}`)
	}))
	defer srv.Close()
	s := makeService(t, testConfig(), provider.New(srv.URL, "test", time.Second))
	if _, err := s.store.Enqueue(message.Message{ID: "a", Topic: "notify", Time: 100, Message: "hi"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.worker(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		failed, err := s.store.FailedCount()
		if err != nil {
			t.Fatal(err)
		}
		if failed == 1 {
			list, err := s.store.ListFailures(1)
			if err != nil || len(list) != 1 || list[0].Attempts != 5 || calls.Load() != 5 || list[0].Reason != "provider business code 1001" {
				t.Fatal("incorrect rejection policy", list, err, calls.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("repeated business rejection never isolated")
}
