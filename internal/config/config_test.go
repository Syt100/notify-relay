package config

import (
	"os"
	"path/filepath"
	"testing"
)

func defaults(t *testing.T) {
	t.Helper()
	t.Setenv("NTFY_TOKEN", "tk_test")
	t.Setenv("WXPUSHER_SPT", "SPT_test")
}
func TestConfig(t *testing.T) {
	defaults(t)
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if c.MinPriority != 1 || c.MaxPending != 10000 || c.Stale.Seconds() != 180 {
		t.Fatalf("defaults: %+v", c)
	}
}
func TestInvalid(t *testing.T) {
	for _, tc := range []struct{ k, v string }{{"NTFY_TOPICS", "../x"}, {"NTFY_TOPICS", ""}, {"NTFY_TOPICS", "notify,notify"}, {"FORWARD_MIN_PRIORITY", "6"}, {"RETRY_BASE_SECONDS", "0"}, {"NTFY_BASE_URL", "http://secret@example.com"}, {"NTFY_BASE_URL", "http://example.com/?token=secret"}, {"WXPUSHER_API", "ftp://x"}, {"MAX_PENDING", "0"}, {"HEALTH_PORT", "0"}, {"LOG_LEVEL", "oops"}} {
		t.Run(tc.k+tc.v, func(t *testing.T) {
			defaults(t)
			t.Setenv(tc.k, tc.v)
			if _, e := Load(); e == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}
func TestSecretFile(t *testing.T) {
	defaults(t)
	p := filepath.Join(t.TempDir(), "token")
	if e := os.WriteFile(p, []byte("SPT_from_file\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("WXPUSHER_SPT", "")
	t.Setenv("WXPUSHER_SPT_FILE", p)
	c, e := Load()
	if e != nil || c.SPT != "SPT_from_file" {
		t.Fatalf("file: %v", e)
	}
	t.Setenv("WXPUSHER_SPT", "SPT_conflict")
	if _, e = Load(); e == nil {
		t.Fatal("ambiguous secret")
	}
}
