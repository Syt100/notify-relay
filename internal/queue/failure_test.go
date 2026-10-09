package queue

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestFailurePersistsDeduplicatesAndCanBeRequeued(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.bolt")
	s, err := Open(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	m := msg("bad", 100)
	m.Message = "private message body"
	m.ContentType = "text/markdown"
	if _, err = s.Enqueue(m); err != nil {
		t.Fatal(err)
	}
	r, err := s.Next(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Fail(r, "provider HTTP 422", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if ok, err := s.Enqueue(m); err != nil || ok {
		t.Fatal("isolated replay duplicated", ok, err)
	}
	if _, err := s.Enqueue(msg("other", 101)); !errors.Is(err, ErrFull) {
		t.Fatal("failures must count toward capacity", err)
	}
	if cursor, err := s.Cursor("notify"); err != nil || cursor != 100 {
		t.Fatal("capacity advanced cursor", cursor, err)
	}
	list, err := s.ListFailures(100)
	if err != nil || len(list) != 1 || list[0].Attempts != 1 {
		t.Fatal(list, err)
	}
	raw, err := json.Marshal(list)
	if err != nil || strings.Contains(string(raw), "private message body") {
		t.Fatal("body leaked in failure metadata")
	}
	if n, err := s.FailedCount(); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err = s.Requeue("notify", "bad"); err != nil {
		t.Fatal(err)
	}
	if n, err := s.FailedCount(); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	r, err = s.Next(time.Now())
	if err != nil || r == nil || r.Message.Message != m.Message || r.Message.ContentType != "text/markdown" || r.Attempts != 0 || r.Rejections != 0 {
		t.Fatal("requeue changed message", r, err)
	}
	if err = s.Accept(r, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = s.Requeue("notify", "bad"); err == nil {
		t.Fatal("missing failure requeued")
	}
	if _, err = s.ListFailures(101); err == nil {
		t.Fatal("unbounded listing allowed")
	}
}

func TestQueueUpgradePreventsOldBinaryIgnoringFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.bolt")
	s, err := Open(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error { return bucket(tx, "meta").Put([]byte("version"), []byte("1")) })
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.db.View(func(tx *bolt.Tx) error {
		if string(bucket(tx, "meta").Get([]byte("version"))) != "2" {
			t.Error("old readers must reject the upgraded queue schema")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
