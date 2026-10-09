package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Syt100/notify-relay/internal/message"
)

func TestMarkdownRejectionFallsBackToText(t *testing.T) {
	var sent []message.Push
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p message.Push
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Error(err)
		}
		sent = append(sent, p)
		if p.ContentType == 3 {
			fmt.Fprint(w, `{"code":1001,"msg":"SPT_secret"}`)
			return
		}
		fmt.Fprint(w, `{"code":1000}`)
	}))
	defer srv.Close()
	var m message.Message
	json.Unmarshal([]byte(`{"message":"**hi**","content_type":"text/markdown"}`), &m)
	if err := New(srv.URL, "SPT_secret", time.Second).Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0].ContentType != 3 || sent[1].ContentType != 1 || !strings.Contains(sent[1].Content, "**hi**") {
		t.Fatalf("expected Markdown followed by text, got %+v", sent)
	}
}

func TestAuthenticationDoesNotTriggerFormatFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
	defer srv.Close()
	var m message.Message
	json.Unmarshal([]byte(`{"message":"**hi**","content_type":"text/markdown"}`), &m)
	if err := New(srv.URL, "SPT_secret", time.Second).Send(context.Background(), m); err == nil {
		t.Fatal("authentication failure accepted")
	}
	if calls != 1 {
		t.Fatal("authentication failure must not resend text")
	}
}

func TestMarkdownRejectionHonorsRetryAfter(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "2")
		fmt.Fprint(w, `{"code":1001}`)
	}))
	defer srv.Close()
	var m message.Message
	json.Unmarshal([]byte(`{"message":"**hi**","content_type":"text/markdown"}`), &m)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := New(srv.URL, "SPT_test", 200*time.Millisecond).Send(ctx, m)
	var delivery *DeliveryError
	if !errors.As(err, &delivery) || delivery.After != 2*time.Second || calls != 1 {
		t.Fatal("fallback must defer to provider cooldown", err, calls)
	}
}

func TestMarkdownFallbackWaitsForCooldown(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "2")
			fmt.Fprint(w, `{"code":1001}`)
			return
		}
		fmt.Fprint(w, `{"code":1000}`)
	}))
	defer srv.Close()
	var m message.Message
	json.Unmarshal([]byte(`{"message":"**hi**","content_type":"text/markdown"}`), &m)
	start := time.Now()
	if err := New(srv.URL, "test", time.Second).Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || time.Since(start) < 2*time.Second {
		t.Fatal("text fallback did not respect cooldown")
	}
}
