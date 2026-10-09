package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syt100/notify-relay/internal/message"
	"github.com/Syt100/notify-relay/internal/queue"
)

func TestProbe(t *testing.T) {
	for _, status := range []int{200, 503} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				t.Error(r.URL.Path)
			}
			w.WriteHeader(status)
		}))
		_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
		t.Setenv("HEALTH_PORT", port)
		e := probe("/healthz")
		srv.Close()
		if (e == nil) != (status == 200) {
			t.Fatal(status, e)
		}
	}
}
func TestCLICommands(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	for _, arg := range []string{"version", "help", "unknown"} {
		os.Args = []string{"bridge", arg}
		e := run()
		if (e != nil) != (arg == "unknown") {
			t.Fatal(arg, e)
		}
	}
}
func TestMissingConfig(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"bridge"}
	t.Setenv("NTFY_TOKEN", "")
	t.Setenv("NTFY_TOKEN_FILE", "")
	if e := run(); e == nil {
		t.Fatal("missing credentials accepted")
	}
}

func TestFailureListCommand(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	t.Setenv("NTFY_TOKEN", "tk_test")
	t.Setenv("NTFY_TOKEN_FILE", "")
	t.Setenv("WXPUSHER_SPT", "SPT_test")
	t.Setenv("WXPUSHER_SPT_FILE", "")
	t.Setenv("STATE_DB", filepath.Join(t.TempDir(), "state.bolt"))
	os.Args = []string{"bridge", "failed", "list"}
	if err := run(); err != nil {
		t.Fatal(err)
	}
}

func TestFailureRetryCommand(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	t.Setenv("NTFY_TOKEN", "tk_test")
	t.Setenv("NTFY_TOKEN_FILE", "")
	t.Setenv("WXPUSHER_SPT", "SPT_test")
	t.Setenv("WXPUSHER_SPT_FILE", "")
	path := filepath.Join(t.TempDir(), "state.bolt")
	t.Setenv("STATE_DB", path)
	q, err := queue.Open(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.Enqueue(message.Message{ID: "a", Topic: "notify", Time: 100, Message: "hi", ContentType: "text/markdown"}); err != nil {
		t.Fatal(err)
	}
	r, err := q.Next(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Fail(r, "provider HTTP 422", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = q.Close(); err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"bridge", "failed", "retry", "notify", "a"}
	if err = run(); err != nil {
		t.Fatal(err)
	}
	q, err = queue.Open(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	r, err = q.Next(time.Now())
	if err != nil || r == nil || r.Message.ID != "a" || r.Message.ContentType != "text/markdown" {
		t.Fatal("CLI requeue lost payload", r, err)
	}
	for _, args := range [][]string{{"failed"}, {"failed", "retry"}, {"failed", "list", "extra"}, {"failed", "unknown"}} {
		os.Args = append([]string{"bridge"}, args...)
		if err = run(); err == nil {
			t.Fatal("invalid maintenance args accepted", args)
		}
	}
}
