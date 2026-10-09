package queue

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syt100/notify-relay/internal/message"
)

func TestBatchedCleanupAndDatabaseGuards(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	s, e := Open(p, 1000)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	for i := 0; i < 300; i++ {
		m := msg(string(rune(i+1000)), 10)
		if _, e = s.Enqueue(m); e != nil {
			t.Fatal(e)
		}
		r, e := s.Next(now)
		if e != nil || r == nil {
			t.Fatal(r, e)
		}
		if e = s.Accept(r, now); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Cleanup(now.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	n, d, e := s.Counts()
	if e != nil || n != 0 || d != 0 {
		t.Fatal(n, d, e)
	}
	_ = s.Close()
	sqlite := filepath.Join(t.TempDir(), "old.db")
	if e = os.WriteFile(sqlite, []byte("SQLite format 3\x00"), 0600); e != nil {
		t.Fatal(e)
	}
	if s, e = Open(sqlite, 10); e == nil {
		_ = s.Close()
		t.Fatal("SQLite accepted")
	}
}

func msg(id string, ts int64) message.Message {
	return message.Message{ID: id, Topic: "notify", Time: ts, Event: "message", Message: "hello"}
}
func TestRestartDedupRetryAndRetention(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	s, e := Open(p, 10)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Checkpoint("notify", 100); e != nil {
		t.Fatal(e)
	}
	if ok, e := s.Enqueue(msg("a", 101)); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(p, 10)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if ok, e := s.Enqueue(msg("a", 101)); e != nil || ok {
		t.Fatal("duplicate", ok, e)
	}
	if ts, e := s.Cursor("notify"); e != nil || ts != 101 {
		t.Fatal(ts, e)
	}
	now := time.Now()
	r, e := s.Next(now)
	if e != nil || r == nil || r.Message.ID != "a" {
		t.Fatal(r, e)
	}
	if e = s.Retry(r, now.Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	if r, e = s.Next(now); e != nil || r != nil {
		t.Fatal("premature retry", r, e)
	}
	r, e = s.Next(now.Add(2 * time.Hour))
	if e != nil || r == nil || r.Attempts != 1 {
		t.Fatal(r, e)
	}
	if e = s.Accept(r, now); e != nil {
		t.Fatal(e)
	}
	if ok, e := s.Enqueue(msg("a", 101)); e != nil || ok {
		t.Fatal("accepted replay", ok, e)
	}
	n, a, e := s.Counts()
	if e != nil || n != 0 || a != 1 {
		t.Fatal(n, a, e)
	}
	if e = s.Cleanup(now.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	_, a, e = s.Counts()
	if e != nil || a != 0 {
		t.Fatal(a, e)
	}
}
func TestCapacityDoesNotAdvanceCursor(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state.db"), 1)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.Enqueue(msg("a", 10)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Enqueue(msg("b", 20)); !errors.Is(e, ErrFull) {
		t.Fatal(e)
	}
	ts, e := s.Cursor("notify")
	if e != nil || ts != 10 {
		t.Fatal("lost checkpoint", ts, e)
	}
	if _, e = s.Enqueue(msg("a", 10)); e != nil {
		t.Fatal("duplicates should not be blocked by capacity", e)
	}
}
func TestSameIDAcrossTopics(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state.db"), 10)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, topic := range []string{"a", "b"} {
		m := msg("same", 10)
		m.Topic = topic
		if ok, e := s.Enqueue(m); e != nil || !ok {
			t.Fatal(ok, e)
		}
	}
	n, _, e := s.Counts()
	if e != nil || n != 2 {
		t.Fatal(n, e)
	}
}
