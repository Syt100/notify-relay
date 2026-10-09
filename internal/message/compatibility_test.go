package message

import (
	"encoding/json"
	"html"
	"strings"
	"testing"
)

func TestMarkdownEventPreservesFormat(t *testing.T) {
	var m Message
	if err := json.Unmarshal([]byte(`{"topic":"backup","title":"结果","message":"**完成**","content_type":"text/markdown"}`), &m); err != nil {
		t.Fatal(err)
	}
	p := Payload(m, "test")
	if p.ContentType != 3 || !strings.Contains(p.Content, "**完成**") {
		t.Fatalf("Markdown format lost: %+v", p)
	}
	if !strings.Contains(p.Summary, "[backup]") || !strings.Contains(p.Content, "来源: backup") {
		t.Fatalf("message source lost: %+v", p)
	}
}

func TestTextBoundsIncludeLineBreakExpansion(t *testing.T) {
	p := Payload(Message{Message: strings.Repeat("x\n", 30000)}, "test")
	expanded := len(html.EscapeString(p.Content)) + 4*strings.Count(p.Content, "\n")
	if expanded > 60000 {
		t.Fatalf("rendered text can exceed provider limit: %d", expanded)
	}
}

func TestOversizeClickPreservedWithoutInvalidURLField(t *testing.T) {
	link := "https://example.com/" + strings.Repeat("x", 1100)
	p := Payload(Message{Click: link, Message: "hi"}, "test")
	if p.URL != "" || !strings.Contains(p.Content, link) {
		t.Fatal("oversize click must be retained in body instead of provider URL field")
	}
}

func TestSourceSurvivesLongTitle(t *testing.T) {
	p := Payload(Message{Topic: "security", Title: strings.Repeat("中", 120), Message: "hi", Priority: 5}, "test")
	if !strings.HasPrefix(p.Summary, "🚨 [security] ") || !strings.Contains(p.Content, "来源: security") {
		t.Fatal("source must survive title truncation")
	}
}

func TestMarkdownMetadataEscapesWithoutBreakingEntities(t *testing.T) {
	p := Payload(Message{Topic: "backup", ContentType: "text/markdown", Message: "```\nunclosed", Attachment: Attachment{Name: `report "Q4" [old].txt`, URL: "https://example.com/log"}}, "test")
	if strings.Contains(p.Content, `&\#34;`) || !strings.Contains(p.Content, `&#34;Q4&#34;`) || !strings.Contains(p.Content, `\[old\]`) {
		t.Fatal("metadata escape changes displayed characters", p.Content)
	}
	if strings.Index(p.Content, "附件:") > strings.Index(p.Content, "```") {
		t.Fatal("unclosed body fence swallowed metadata")
	}
}
