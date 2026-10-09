// Package queue provides an fsync-backed transactional disk queue.
package queue

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/Syt100/notify-relay/internal/message"
)

var ErrFull = errors.New("pending queue is full")
var buckets = []string{"meta", "cursor", "pending", "due", "seen", "retained"}

type Store struct {
	db  *bolt.DB
	max int
}
type Record struct {
	Message  message.Message `json:"message"`
	Attempts int             `json:"attempts"`
	Due      int64           `json:"due"`
}

func key(m message.Message) []byte              { return []byte(m.Topic + "\x00" + m.ID) }
func number(n int64) []byte                     { b := make([]byte, 8); binary.BigEndian.PutUint64(b, uint64(n)); return b }
func index(n int64, k []byte) []byte            { return append(number(n), k...) }
func bucket(tx *bolt.Tx, n string) *bolt.Bucket { return tx.Bucket([]byte(n)) }
func count(tx *bolt.Tx, name string) int64 {
	v := bucket(tx, "meta").Get([]byte(name + "_count"))
	if v == nil {
		return 0
	}
	return int64(binary.BigEndian.Uint64(v))
}
func changeCount(tx *bolt.Tx, name string, delta int64) error {
	n := count(tx, name) + delta
	if n < 0 {
		return fmt.Errorf("negative queue count")
	}
	return bucket(tx, "meta").Put([]byte(name+"_count"), number(n))
}
func Open(path string, max int) (*Store, error) {
	if max < 1 {
		return nil, fmt.Errorf("queue capacity must be positive")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	// Refuse the former SQLite file rather than passing it to bbolt's mmap reader.
	if f, e := os.Open(path); e == nil {
		var header [16]byte
		n, _ := f.Read(header[:])
		_ = f.Close()
		if n >= 15 && string(header[:15]) == "SQLite format 3" {
			return nil, fmt.Errorf("STATE_DB is a SQLite file; choose a new .bolt path")
		}
	}
	db, e := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if e != nil {
		return nil, e
	}
	e = db.Update(func(tx *bolt.Tx) error {
		for _, n := range buckets {
			if _, e := tx.CreateBucketIfNotExists([]byte(n)); e != nil {
				return e
			}
		}
		meta := bucket(tx, "meta")
		v := meta.Get([]byte("version"))
		if v != nil && string(v) != "1" {
			return fmt.Errorf("unsupported queue schema")
		}
		for _, name := range []string{"pending", "seen"} {
			if meta.Get([]byte(name+"_count")) == nil {
				if e := meta.Put([]byte(name+"_count"), number(int64(bucket(tx, name).Stats().KeyN))); e != nil {
					return e
				}
			}
		}
		return meta.Put([]byte("version"), []byte("1"))
	})
	if e != nil {
		_ = db.Close()
		return nil, e
	}
	return &Store{db: db, max: max}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func advance(tx *bolt.Tx, topic string, ts int64) error {
	b := bucket(tx, "cursor")
	old := b.Get([]byte(topic))
	if old != nil && int64(binary.BigEndian.Uint64(old)) >= ts {
		return nil
	}
	return b.Put([]byte(topic), number(ts))
}
func (s *Store) Checkpoint(topic string, ts int64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if bucket(tx, "cursor").Get([]byte(topic)) != nil {
			return nil
		}
		return advance(tx, topic, ts)
	})
}
func (s *Store) Cursor(topic string) (int64, error) {
	var n int64
	e := s.db.View(func(tx *bolt.Tx) error {
		v := bucket(tx, "cursor").Get([]byte(topic))
		if v != nil {
			n = int64(binary.BigEndian.Uint64(v))
		}
		return nil
	})
	return n, e
}
func (s *Store) Enqueue(m message.Message) (bool, error) {
	if m.ID == "" || m.Topic == "" || m.Time <= 0 {
		return false, fmt.Errorf("invalid ntfy message identity or time")
	}
	inserted := false
	e := s.db.Update(func(tx *bolt.Tx) error {
		k := key(m)
		pending := bucket(tx, "pending")
		if pending.Get(k) == nil && bucket(tx, "seen").Get(k) == nil {
			if count(tx, "pending") >= int64(s.max) {
				return ErrFull
			}
			raw, e := json.Marshal(Record{Message: m})
			if e != nil {
				return e
			}
			if e = pending.Put(k, raw); e != nil {
				return e
			}
			if e = bucket(tx, "due").Put(index(0, k), k); e != nil {
				return e
			}
			if e = changeCount(tx, "pending", 1); e != nil {
				return e
			}
			inserted = true
		}
		return advance(tx, m.Topic, m.Time)
	})
	return inserted, e
}
func (s *Store) Next(now time.Time) (*Record, error) {
	var r *Record
	e := s.db.View(func(tx *bolt.Tx) error {
		k, v := bucket(tx, "due").Cursor().First()
		if k == nil || int64(binary.BigEndian.Uint64(k[:8])) > now.UnixNano() {
			return nil
		}
		raw := bucket(tx, "pending").Get(v)
		if raw == nil {
			return fmt.Errorf("queue index references missing record")
		}
		r = &Record{}
		return json.Unmarshal(raw, r)
	})
	return r, e
}
func (s *Store) Retry(r *Record, next time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		k := key(r.Message)
		if bucket(tx, "pending").Get(k) == nil {
			return fmt.Errorf("pending record is missing")
		}
		if e := bucket(tx, "due").Delete(index(r.Due, k)); e != nil {
			return e
		}
		updated := *r
		updated.Due = next.UnixNano()
		updated.Attempts++
		raw, e := json.Marshal(updated)
		if e != nil {
			return e
		}
		if e = bucket(tx, "pending").Put(k, raw); e != nil {
			return e
		}
		return bucket(tx, "due").Put(index(updated.Due, k), k)
	})
}
func (s *Store) Accept(r *Record, now time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		k := key(r.Message)
		if bucket(tx, "pending").Get(k) == nil {
			return fmt.Errorf("pending record is missing")
		}
		if e := bucket(tx, "pending").Delete(k); e != nil {
			return e
		}
		if e := bucket(tx, "due").Delete(index(r.Due, k)); e != nil {
			return e
		}
		if e := bucket(tx, "seen").Put(k, number(now.UnixNano())); e != nil {
			return e
		}
		if e := changeCount(tx, "pending", -1); e != nil {
			return e
		}
		if e := changeCount(tx, "seen", 1); e != nil {
			return e
		}
		return bucket(tx, "retained").Put(index(now.UnixNano(), k), k)
	})
}
func (s *Store) Counts() (int, int, error) {
	var p, d int
	e := s.db.View(func(tx *bolt.Tx) error {
		p = int(count(tx, "pending"))
		d = int(count(tx, "seen"))
		return nil
	})
	return p, d, e
}
func (s *Store) Cleanup(cutoff time.Time) error {
	// Small transactions keep cleanup bounded even after a long downtime.
	for {
		removed := 0
		e := s.db.Update(func(tx *bolt.Tx) error {
			c := bucket(tx, "retained").Cursor()
			for k, v := c.First(); k != nil && removed < 256; k, v = c.Next() {
				if int64(binary.BigEndian.Uint64(k[:8])) >= cutoff.UnixNano() {
					break
				}
				if e := bucket(tx, "seen").Delete(v); e != nil {
					return e
				}
				if e := c.Delete(); e != nil {
					return e
				}
				if e := changeCount(tx, "seen", -1); e != nil {
					return e
				}
				removed++
			}
			return nil
		})
		if e != nil {
			return e
		}
		if removed < 256 {
			return nil
		}
	}
}
