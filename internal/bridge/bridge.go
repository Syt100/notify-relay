// Package bridge coordinates subscriptions, the persistent outbox and delivery.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Syt100/notify-relay/internal/config"
	"github.com/Syt100/notify-relay/internal/message"
	"github.com/Syt100/notify-relay/internal/provider"
	"github.com/Syt100/notify-relay/internal/queue"
)

type Sender interface {
	Send(context.Context, message.Message) error
}
type activity struct {
	Connected       bool      `json:"connected"`
	Last            time.Time `json:"last_activity"`
	ReplayTruncated bool      `json:"replay_truncated"`
}
type Service struct {
	cfg           config.Config
	store         *queue.Store
	sender        Sender
	log           *slog.Logger
	client        *http.Client
	mu            sync.Mutex
	topics        map[string]activity
	providerError string
	storageError  bool
}

func New(c config.Config, q *queue.Store, p Sender, l *slog.Logger) *Service {
	topics := map[string]activity{}
	for _, t := range c.Topics {
		topics[t] = activity{}
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxIdleConns: len(c.Topics) + 1, IdleConnTimeout: 90 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Service{cfg: c, store: q, sender: p, log: l, client: client, topics: topics}
}
func wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func (s *Service) activity(topic string, connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.topics[topic]
	a.Connected = connected
	if connected {
		a.Last = time.Now()
	}
	s.topics[topic] = a
}

type storageFailure struct{ error }

func (s *Service) subscribe(ctx context.Context, topic string) error {
	ts, e := s.store.Cursor(topic)
	if e != nil {
		return storageFailure{e}
	}
	u, e := url.Parse(s.cfg.BaseURL)
	if e != nil {
		return fmt.Errorf("invalid ntfy URL")
	}
	u.Path = u.Path + "/" + topic + "/json"
	if ts > 0 {
		q := u.Query()
		since := ts - 1
		if since < 1 {
			since = 1
		}
		q.Set("since", strconv.FormatInt(since, 10))
		u.RawQuery = q.Encode()
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, e := http.NewRequestWithContext(streamCtx, "GET", u.String(), nil)
	if e != nil {
		return fmt.Errorf("invalid subscription request")
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	req.Header.Set("User-Agent", "notify-relay")
	resp, e := s.client.Do(req)
	if e != nil {
		return fmt.Errorf("ntfy connection failed or timed out")
	}
	defer resp.Body.Close()
	defer s.activity(topic, false)
	if resp.StatusCode != 200 {
		return fmt.Errorf("ntfy HTTP %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Messages-Truncated") == "1" {
		s.mu.Lock()
		a := s.topics[topic]
		a.ReplayTruncated = true
		s.topics[topic] = a
		s.mu.Unlock()
		s.log.Error("ntfy replay truncated: older cached messages were omitted", "topic", topic)
	}
	// A stale stream is actively cancelled, rather than merely marked unhealthy.
	watchdog := time.AfterFunc(s.cfg.Stale, cancel)
	defer watchdog.Stop()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var m message.Message
		if e = json.Unmarshal(scanner.Bytes(), &m); e != nil {
			return fmt.Errorf("invalid ntfy JSON event")
		}
		if m.Topic != topic {
			return fmt.Errorf("ntfy topic mismatch")
		}
		if m.Event != "open" && m.Event != "keepalive" && m.Event != "message" {
			continue
		}
		watchdog.Reset(s.cfg.Stale)
		switch m.Event {
		case "open":
			if m.Time <= 0 {
				return fmt.Errorf("invalid ntfy open timestamp")
			}
			if e = s.store.Checkpoint(topic, m.Time); e != nil {
				return storageFailure{e}
			}
		case "message":
			inserted, er := s.store.Enqueue(m)
			if er != nil {
				if errors.Is(er, queue.ErrFull) {
					return er
				}
				return storageFailure{er}
			}
			if inserted {
				s.log.Debug("queued", "topic", topic, "id", m.ID)
			}
		}
		s.activity(topic, true)
	}
	if scanner.Err() != nil {
		return fmt.Errorf("ntfy stream interrupted or event exceeds 1 MiB")
	}
	return io.EOF
}
func (s *Service) subscriber(ctx context.Context, topic string) error {
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		e := s.subscribe(ctx, topic)
		if ctx.Err() != nil {
			return nil
		}
		var disk storageFailure
		if errors.As(e, &disk) {
			return fmt.Errorf("subscription storage failure: %w", disk.error)
		}
		s.activity(topic, false)
		if time.Since(start) > s.cfg.Stale {
			backoff = time.Second
		}
		delay := backoff
		if errors.Is(e, queue.ErrFull) {
			delay = time.Second
		}
		s.log.Warn("subscription interrupted", "topic", topic, "reason", e.Error(), "retry_seconds", delay.Seconds())
		if !wait(ctx, delay) {
			return nil
		}
		if backoff < 30*time.Second {
			backoff *= 2
		} else {
			backoff = 60 * time.Second
		}
	}
	return nil
}
func retryDelay(base, max time.Duration, attempt int) time.Duration {
	d := base
	for i := 0; i < attempt && d < max; i++ {
		if d > max/2 {
			d = max
			break
		}
		d *= 2
	}
	if d >= max {
		return max
	}
	jitter := time.Duration(rand.Float64() * 0.2 * float64(d))
	if d > max-jitter {
		return max
	}
	return d + jitter
}
func (s *Service) worker(ctx context.Context) error {
	nextSend := time.Time{}
	nextCleanup := time.Time{}
	for ctx.Err() == nil {
		now := time.Now()
		if now.After(nextCleanup) {
			if e := s.store.Cleanup(now.Add(-s.cfg.Retention)); e != nil {
				return e
			}
			nextCleanup = now.Add(time.Hour)
		}
		r, e := s.store.Next(now)
		if e != nil {
			return e
		}
		if r == nil {
			if !wait(ctx, 250*time.Millisecond) {
				return nil
			}
			continue
		}
		if r.Message.Level() < s.cfg.MinPriority {
			if e = s.store.Accept(r, now); e != nil {
				return e
			}
			s.log.Debug("filtered", "topic", r.Message.Topic, "id", r.Message.ID)
			continue
		}
		if !wait(ctx, time.Until(nextSend)) {
			return nil
		}
		nextSend = time.Now().Add(time.Second)
		e = s.sender.Send(ctx, r.Message)
		// Send may include a paced Markdown-to-text fallback request.
		nextSend = time.Now().Add(time.Second)
		if ctx.Err() != nil {
			return nil
		}
		s.mu.Lock()
		if e != nil {
			s.providerError = e.Error()
		} else {
			s.providerError = ""
		}
		s.mu.Unlock()
		if e == nil {
			if e = s.store.Accept(r, time.Now()); e != nil {
				return e
			}
			s.log.Info("accepted", "topic", r.Message.Topic, "id", r.Message.ID)
			continue
		}
		delay := retryDelay(s.cfg.RetryBase, s.cfg.RetryMax, r.Attempts)
		var delivery *provider.DeliveryError
		if errors.As(e, &delivery) {
			if delivery.After > 0 {
				if delivery.After > delay {
					delay = delivery.After
				}
				cooldown := time.Now().Add(delivery.After)
				if cooldown.After(nextSend) {
					nextSend = cooldown
				}
			}
			if delivery.Rejected {
				r.Rejections++
			} else {
				r.Rejections = 0
			}
			if delivery.Permanent || r.Rejections >= 5 {
				if er := s.store.Fail(r, delivery.Reason, time.Now()); er != nil {
					return er
				}
				s.log.Error("delivery isolated", "topic", r.Message.Topic, "id", r.Message.ID, "reason", delivery.Reason)
				continue
			}
		}
		if er := s.store.Retry(r, time.Now().Add(delay)); er != nil {
			return er
		}
		s.log.Warn("delivery failed", "topic", r.Message.Topic, "id", r.Message.ID, "reason", e.Error(), "attempt", r.Attempts+1, "retry_seconds", delay.Seconds())
	}
	return nil
}
func (s *Service) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 1)
	launch := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := fn(); e != nil {
				select {
				case errs <- e:
				default:
				}
				cancel()
			}
		}()
	}
	for _, topic := range s.cfg.Topics {
		launch(func() error { return s.subscriber(ctx, topic) })
	}
	launch(func() error { return s.worker(ctx) })
	<-ctx.Done()
	wg.Wait()
	s.client.CloseIdleConnections()
	select {
	case e := <-errs:
		s.mu.Lock()
		s.storageError = true
		s.mu.Unlock()
		return e
	default:
		return nil
	}
}
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		p, d, e := s.store.Counts()
		failed, failedErr := s.store.FailedCount()
		s.mu.Lock()
		topics := make(map[string]activity, len(s.topics))
		ready := e == nil && failedErr == nil && failed == 0 && !s.storageError && s.providerError == ""
		for t, a := range s.topics {
			topics[t] = a
			if !a.Connected || a.ReplayTruncated || time.Since(a.Last) > s.cfg.Stale {
				ready = false
			}
		}
		live := e == nil && failedErr == nil && !s.storageError
		providerError := s.providerError
		s.mu.Unlock()
		healthy := live
		if r.URL.Path == "/readyz" {
			healthy = ready
		}
		w.Header().Set("Content-Type", "application/json")
		if !healthy {
			w.WriteHeader(503)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"healthy": healthy, "ready": ready, "topics": topics, "pending": p, "failed": failed, "processed_retained": d, "provider_error": providerError})
	}
	mux.HandleFunc("/healthz", handler)
	mux.HandleFunc("/readyz", handler)
	return mux
}
