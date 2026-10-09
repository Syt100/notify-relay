package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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
