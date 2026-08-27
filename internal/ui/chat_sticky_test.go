package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestFlattenOneLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"hello", "hello"},
		{"hello world", "hello world"},
		{"multi\nline\nprompt", "multi line prompt"},
		{"  leading and   collapsed\tspaces ", "leading and collapsed spaces"},
		{"trailing newline\n", "trailing newline"},
	}
	for _, c := range cases {
		if got := flattenOneLine(c.in); got != c.want {
			t.Errorf("flattenOneLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncateOneLine(t *testing.T) {
	cases := []struct {
		in       string
		maxWidth int
		want     string
	}{
		{"hello", 0, ""},
		{"hello", -3, ""},
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hell…"},
		{"hello world", 1, "…"},
	}
	for _, c := range cases {
		if got := truncateOneLine(c.in, c.maxWidth); got != c.want {
			t.Errorf("truncateOneLine(%q, %d) = %q, want %q", c.in, c.maxWidth, got, c.want)
		}
	}
}

// TestUserMessageInfos verifies that each user message's recorded line index
// points at the line where its rendered block starts — even when grouped file
// operations between prompts shift the offsets — and that the text is flattened.
func TestUserMessageInfos(t *testing.T) {
	s := NewStyles(true)
	const width = 60

	msgs := []ChatMessage{
		{Type: MsgUser, Text: "hello   world", Rendered: "u1\n"},
		// Two consecutive read_file calls on the same file force grouping,
		// which changes the rendered line count between the prompts.
		{Type: MsgToolCall, ToolName: "read_file", FilePath: "foo.go", Text: "foo.go", Rendered: "tc1\n"},
		{Type: MsgToolResult, ToolName: "read_file", Text: "10 lines", Rendered: "tr1\n"},
		{Type: MsgToolCall, ToolName: "read_file", FilePath: "foo.go", Text: "foo.go", Rendered: "tc2\n"},
		{Type: MsgToolResult, ToolName: "read_file", Text: "12 lines", Rendered: "tr2\n"},
		{Type: MsgAssistant, Rendered: "a1\n"},
		{Type: MsgSystem, TurnModel: "m", Rendered: "sep1\n"},
		{Type: MsgUser, Text: "second\nprompt  here", Rendered: "u2\n"},
	}

	lines := strings.Split(buildRenderedChat(msgs, s, width), "\n")
	infos := userMessageInfos(msgs, s, width)

	if len(infos) != 2 {
		t.Fatalf("got %d user infos, want 2", len(infos))
	}

	want := []struct {
		marker string
		text   string
		msgIdx int
	}{
		{"u1", "hello world", 0},
		{"u2", "second prompt here", 7},
	}
	for i, w := range want {
		if infos[i].LineIdx < 0 || infos[i].LineIdx >= len(lines) {
			t.Fatalf("info[%d].LineIdx = %d out of range (%d lines)", i, infos[i].LineIdx, len(lines))
		}
		if got := lines[infos[i].LineIdx]; got != w.marker {
			t.Errorf("info[%d]: line at LineIdx %d = %q, want %q", i, infos[i].LineIdx, got, w.marker)
		}
		if infos[i].Text != w.text {
			t.Errorf("info[%d].Text = %q, want %q", i, infos[i].Text, w.text)
		}
		if infos[i].MsgIdx != w.msgIdx {
			t.Errorf("info[%d].MsgIdx = %d, want %d", i, infos[i].MsgIdx, w.msgIdx)
		}
	}
}

func TestStickyUserPromptForTop(t *testing.T) {
	infos := []UserMsgInfo{
		{LineIdx: 0, Text: "one"},
		{LineIdx: 5, Text: "two"},
		{LineIdx: 10, Text: "three"},
	}
	cases := []struct {
		topLine int
		want    string
	}{
		{0, "one"},
		{4, "one"},
		{5, "two"},
		{9, "two"},
		{10, "three"},
		{1000, "three"},
	}
	for _, c := range cases {
		if got := stickyUserPromptForTop(infos, c.topLine); got != c.want {
			t.Errorf("stickyUserPromptForTop(topLine=%d) = %q, want %q", c.topLine, got, c.want)
		}
	}

	// No user message at or above the top line yields no header.
	shifted := []UserMsgInfo{{LineIdx: 3, Text: "later"}}
	if got := stickyUserPromptForTop(shifted, 0); got != "" {
		t.Errorf("stickyUserPromptForTop above first prompt = %q, want \"\"", got)
	}
	if got := stickyUserPromptForTop(nil, 0); got != "" {
		t.Errorf("stickyUserPromptForTop(nil) = %q, want \"\"", got)
	}
}

// TestRenderStickyHeader checks the header is a single text line plus a
// full-width rule, and that an over-long prompt is truncated to one line.
func TestRenderStickyHeader(t *testing.T) {
	const innerWidth = 20
	long := strings.Repeat("word ", 30)
	header := renderStickyHeader(long, innerWidth)

	parts := strings.Split(header, "\n")
	if len(parts) != 2 {
		t.Fatalf("header has %d lines, want 2: %q", len(parts), header)
	}
	if w := lipgloss.Width(parts[0]); w > innerWidth {
		t.Errorf("header text line width = %d, want <= %d", w, innerWidth)
	}
	if w := lipgloss.Width(parts[1]); w != innerWidth {
		t.Errorf("separator width = %d, want %d", w, innerWidth)
	}
}
