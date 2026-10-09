// Package message translates ntfy events into bounded WxPusher payloads.
package message

import (
	"html"
	"strings"
)

type Attachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type Action struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
}
type Message struct {
	ID          string     `json:"id"`
	Time        int64      `json:"time"`
	Event       string     `json:"event"`
	Topic       string     `json:"topic"`
	Title       string     `json:"title,omitempty"`
	Message     string     `json:"message,omitempty"`
	ContentType string     `json:"content_type,omitempty"`
	Priority    int        `json:"priority,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Click       string     `json:"click,omitempty"`
	Attachment  Attachment `json:"attachment,omitempty"`
	Actions     []Action   `json:"actions,omitempty"`
}

func (m Message) Level() int {
	if m.Priority < 1 || m.Priority > 5 {
		return 3
	}
	return m.Priority
}

type Push struct {
	Content     string `json:"content"`
	Summary     string `json:"summary"`
	ContentType int    `json:"contentType"`
	SPT         string `json:"spt"`
	URL         string `json:"url,omitempty"`
}

func cut(s string, n int) string {
	if units(s) <= n {
		return s
	}
	used := 0
	for i, r := range s {
		cost := 1
		if r > 0xffff {
			cost = 2
		}
		if used+cost > n-1 {
			return s[:i] + "…"
		}
		used += cost
	}
	return s
}

// UTF-16 units also satisfy providers that count surrogate pairs separately.
func units(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}

// Bound escaped text and newline-to-HTML expansion with room for wrappers.
// Markdown rendering is provider-specific; Send falls back to bounded text
// if the provider rejects its rendered result.
func BoundText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = cut(s, 40000)
	budget := 60000
	used := 0
	for i, r := range s {
		cost := len(html.EscapeString(string(r)))
		if r == '\n' {
			cost = 6
		}
		if used+cost > budget-3 {
			return s[:i] + "…"
		}
		used += cost
	}
	return s
}

func markdownText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\`*_{}[]()#+-.!|~", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return html.EscapeString(b.String())
}

func Payload(m Message, spt string) Push {
	title := m.Title
	if title == "" {
		title = m.Topic
	}
	if title == "" {
		title = "ntfy"
	}
	if m.Topic != "" && m.Title != "" {
		title = "[" + m.Topic + "] " + title
	}
	prefix := map[int]string{1: "💤 ", 2: "ℹ️ ", 4: "⚠️ ", 5: "🚨 "}[m.Level()]
	metadata := []string{}
	if m.Topic != "" {
		metadata = append(metadata, "来源: "+m.Topic)
	}
	if len(m.Tags) > 0 {
		metadata = append(metadata, "标签: "+strings.Join(m.Tags, ", "))
	}
	if m.Attachment.URL != "" {
		metadata = append(metadata, "附件: "+m.Attachment.Name+"\n"+m.Attachment.URL)
	}
	for _, a := range m.Actions {
		if a.Action == "view" && a.URL != "" {
			metadata = append(metadata, a.Label+": "+a.URL)
		}
	}
	link := strings.TrimSpace(m.Click)
	if units(link) > 1000 {
		metadata = append(metadata, "原文: "+link)
		link = ""
	}
	contentType := 1
	if strings.EqualFold(strings.TrimSpace(strings.Split(m.ContentType, ";")[0]), "text/markdown") {
		contentType = 3
		for i := range metadata {
			metadata[i] = markdownText(metadata[i])
		}
	}
	// Put metadata before the body so an unclosed Markdown code fence cannot
	// swallow it. The original message itself remains Markdown, unescaped.
	parts := metadata
	if m.Message != "" {
		parts = append(parts, m.Message)
	}
	content := strings.Join(parts, "\n\n")
	if content == "" {
		content = "(empty message)"
	}
	content = BoundText(content)
	return Push{Content: content, Summary: cut(prefix+title, 100), ContentType: contentType, SPT: spt, URL: link}
}
