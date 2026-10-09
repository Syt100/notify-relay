// Package message translates ntfy events into bounded WxPusher text payloads.
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
	ID         string     `json:"id"`
	Time       int64      `json:"time"`
	Event      string     `json:"event"`
	Topic      string     `json:"topic"`
	Title      string     `json:"title,omitempty"`
	Message    string     `json:"message,omitempty"`
	Priority   int        `json:"priority,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Click      string     `json:"click,omitempty"`
	Attachment Attachment `json:"attachment,omitempty"`
	Actions    []Action   `json:"actions,omitempty"`
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
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
func Payload(m Message, spt string) Push {
	title := m.Title
	if title == "" {
		title = m.Topic
	}
	if title == "" {
		title = "ntfy"
	}
	prefix := map[int]string{1: "💤 ", 2: "ℹ️ ", 4: "⚠️ ", 5: "🚨 "}[m.Level()]
	parts := []string{}
	if m.Message != "" {
		parts = append(parts, m.Message)
	}
	if len(m.Tags) > 0 {
		parts = append(parts, "标签: "+strings.Join(m.Tags, ", "))
	}
	if m.Attachment.URL != "" {
		parts = append(parts, "附件: "+m.Attachment.Name+"\n"+m.Attachment.URL)
	}
	for _, a := range m.Actions {
		if a.Action == "view" && a.URL != "" {
			parts = append(parts, a.Label+": "+a.URL)
		}
	}
	content := strings.Join(parts, "\n\n")
	if content == "" {
		content = "(empty message)"
	}
	content = cut(content, 40000)
	// Text is converted to HTML upstream. Bound the expanded form as well as UTF-8.
	if len(html.EscapeString(content)) > 60000 {
		r := []rune(content)
		lo, hi := 0, len(r)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if len(html.EscapeString(string(r[:mid])+"…")) <= 60000 {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		content = string(r[:lo]) + "…"
	}
	return Push{Content: content, Summary: cut(prefix+title, 100), ContentType: 1, SPT: spt, URL: strings.TrimSpace(m.Click)}
}
