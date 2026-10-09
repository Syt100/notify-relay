package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Syt100/notify-relay/internal/message"
)

func TestSend(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{{"accepted", `{"code":1000,"success":true}`, 200, true}, {"business error", `{"code":1001,"msg":"SPT_secret","success":false}`, 200, false}, {"false success", `{"code":1000,"success":false}`, 200, false}, {"HTTP error", `SPT_secret`, 429, false}, {"bad JSON", `SPT_secret`, 200, false}, {"huge", strings.Repeat("x", 70000), 200, false}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var p message.Push
				if e := json.NewDecoder(r.Body).Decode(&p); e != nil {
					t.Error(e)
				}
				if p.SPT != "SPT_secret" || p.Content != "hi" {
					t.Error("payload")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			p := New(srv.URL, "SPT_secret", time.Second)
			e := p.Send(context.Background(), message.Message{Message: "hi"})
			if (e == nil) != tc.ok {
				t.Fatal(e)
			}
			if e != nil && strings.Contains(e.Error(), "SPT_secret") {
				t.Fatal("secret leaked")
			}
		})
	}
}
func TestRedirectRefused(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer srv.Close()
	if e := New(srv.URL, "secret", time.Second).Send(context.Background(), message.Message{}); e == nil || reached {
		t.Fatal("followed credential-bearing redirect")
	}
}
