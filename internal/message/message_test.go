package message

import (
	"html"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMapping(t *testing.T) {
	p := Payload(Message{Topic: "backup", Title: "备份失败", Message: "磁盘满", Priority: 5, Tags: []string{"backup", "error"}, Click: "https://example.com/status", Attachment: Attachment{Name: "log.txt", URL: "https://example.com/log"}, Actions: []Action{{Action: "view", Label: "详情", URL: "https://example.com/details"}}}, "SPT_test")
	if p.Summary != "🚨 [backup] 备份失败" || p.URL != "https://example.com/status" || p.ContentType != 1 || p.SPT != "SPT_test" {
		t.Fatalf("bad mapping: %+v", p)
	}
	for _, part := range []string{"磁盘满", "backup, error", "log.txt", "https://example.com/details"} {
		if !strings.Contains(p.Content, part) {
			t.Fatalf("missing %q", part)
		}
	}
	if (Message{}).Level() != 3 {
		t.Fatal("default priority")
	}
}

func TestBounds(t *testing.T) {
	for _, text := range []string{strings.Repeat("中😀", 40000), strings.Repeat("<&", 40000)} {
		p := Payload(Message{Title: text, Message: text}, "test")
		if !utf8.ValidString(p.Content) || utf8.RuneCountInString(p.Summary) > 100 || utf8.RuneCountInString(p.Content) > 40000 || len(html.EscapeString(p.Content)) > 60000 {
			t.Fatal("payload exceeds UTF-8/expanded bounds")
		}
	}
}
