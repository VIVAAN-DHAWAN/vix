package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/get-vix/vix/internal/daemon"
	"github.com/get-vix/vix/internal/protocol"
)

// keyPress builds a KeyPressMsg for a named key ("esc") whose String() matches.
func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		r := []rune(name)[0]
		return tea.KeyPressMsg{Code: r, Text: name}
	}
}

// beginRename pre-fills the text box with the current title and opens the
// dialog targeting the right thread.
func TestBeginRename_PrefillsAndOpens(t *testing.T) {
	m := &Model{width: 100}
	m.beginRename(2, "", "Current title")
	if m.state != StateThreadRename {
		t.Fatalf("state = %v, want StateThreadRename", m.state)
	}
	if got := m.renameInput.Value(); got != "Current title" {
		t.Fatalf("input pre-fill = %q, want %q", got, "Current title")
	}
	if m.renameIdx != 2 || m.renameID != "" {
		t.Fatalf("target = (idx=%d,id=%q), want (2,\"\")", m.renameIdx, m.renameID)
	}
}

// r on a live Threads-tab row opens the rename dialog pre-filled with the
// thread's current title.
func TestBeginRenameSelected_LiveThread(t *testing.T) {
	cfg := testCfg(t.TempDir())
	live := newThreadState(cfg, nil)
	live.client = daemon.NewThreadClient(cfg.SocketPath) // non-nil = "connected enough"
	live.title = "Live title"
	m := &Model{cfg: cfg, cwd: cfg.CWD, width: 100, threads: []*ThreadState{live}, threadsSelected: 1}

	m.beginRenameSelected()

	if m.state != StateThreadRename {
		t.Fatalf("state = %v, want StateThreadRename", m.state)
	}
	if m.renameIdx != 0 || m.renameID != "" {
		t.Fatalf("target = (idx=%d,id=%q), want live (0,\"\")", m.renameIdx, m.renameID)
	}
	if got := m.renameInput.Value(); got != "Live title" {
		t.Fatalf("input pre-fill = %q, want %q", got, "Live title")
	}
}

// r on a live row whose client hasn't connected is refused (no dialog).
func TestBeginRenameSelected_ConnectingRefused(t *testing.T) {
	cfg := testCfg(t.TempDir())
	live := newThreadState(cfg, nil) // client == nil
	m := &Model{cfg: cfg, cwd: cfg.CWD, width: 100, threads: []*ThreadState{live}, threadsSelected: 1}
	m.beginRenameSelected()
	if m.state == StateThreadRename {
		t.Fatal("a still-connecting thread must not open the rename dialog")
	}
}

// r on a persisted, not-open record targets it by ID with its current title.
func TestBeginRenameSelected_PersistedRecord(t *testing.T) {
	cfg := testCfg("/work")
	m := &Model{
		cfg: cfg, cwd: "/work", width: 100,
		userThreadRecords: []protocol.ThreadSummary{
			{ID: "rec1", CWD: "/work", Title: "Persisted title", LastRequestAt: "2026-01-01T00:00:00Z"},
		},
		threadsSelected: 1,
	}
	m.beginRenameSelected()
	if m.state != StateThreadRename {
		t.Fatalf("state = %v, want StateThreadRename", m.state)
	}
	if m.renameID != "rec1" || m.renameIdx != -1 {
		t.Fatalf("target = (idx=%d,id=%q), want persisted (-1,\"rec1\")", m.renameIdx, m.renameID)
	}
	if got := m.renameInput.Value(); got != "Persisted title" {
		t.Fatalf("input pre-fill = %q, want %q", got, "Persisted title")
	}
}

// Enter on a live-thread rename updates the title optimistically and closes the
// dialog.
func TestSubmitRename_LiveOptimistic(t *testing.T) {
	cfg := testCfg(t.TempDir())
	live := newThreadState(cfg, nil)
	live.client = daemon.NewThreadClient(cfg.SocketPath)
	live.title = "old"
	m := &Model{cfg: cfg, width: 100, threads: []*ThreadState{live}}
	m.beginRename(0, "", "old")
	m.renameInput.SetValue("brand new name")

	m2, cmd := m.submitRename()
	mm := m2.(Model)
	if mm.state != StateWaitingForInput {
		t.Fatalf("state = %v, want StateWaitingForInput", mm.state)
	}
	if live.title != "brand new name" {
		t.Fatalf("optimistic title = %q, want %q", live.title, "brand new name")
	}
	if cmd == nil {
		t.Fatal("expected a command to send the rename to the daemon")
	}
}

// Enter on a persisted-record rename updates the in-memory list optimistically.
func TestSubmitRename_PersistedOptimistic(t *testing.T) {
	cfg := testCfg("/work")
	m := &Model{
		cfg: cfg, cwd: "/work", width: 100,
		userThreadRecords: []protocol.ThreadSummary{{ID: "rec1", CWD: "/work", Title: "old"}},
	}
	m.beginRename(-1, "rec1", "old")
	m.renameInput.SetValue("new persisted")

	m2, cmd := m.submitRename()
	mm := m2.(Model)
	if mm.userThreadRecords[0].Title != "new persisted" {
		t.Fatalf("optimistic record title = %q, want %q", mm.userThreadRecords[0].Title, "new persisted")
	}
	if cmd == nil {
		t.Fatal("expected a command to send the rename RPC")
	}
}

// An empty title submit is a no-op: dialog closes, title unchanged.
func TestSubmitRename_EmptyIsNoop(t *testing.T) {
	cfg := testCfg(t.TempDir())
	live := newThreadState(cfg, nil)
	live.client = daemon.NewThreadClient(cfg.SocketPath)
	live.title = "keep me"
	m := &Model{cfg: cfg, width: 100, threads: []*ThreadState{live}}
	m.beginRename(0, "", "keep me")
	m.renameInput.SetValue("   ")

	m2, _ := m.submitRename()
	mm := m2.(Model)
	if mm.state != StateWaitingForInput {
		t.Fatalf("state = %v, want StateWaitingForInput", mm.state)
	}
	if live.title != "keep me" {
		t.Fatalf("empty submit changed title to %q", live.title)
	}
}

// Esc cancels the rename without mutating the title.
func TestHandleRenameKey_EscCancels(t *testing.T) {
	cfg := testCfg(t.TempDir())
	live := newThreadState(cfg, nil)
	live.title = "unchanged"
	m := Model{cfg: cfg, width: 100, threads: []*ThreadState{live}}
	m.beginRename(0, "", "unchanged")
	m.renameInput.SetValue("typed but discarded")

	m2, _ := m.handleRenameKey(keyPress("esc"))
	mm := m2.(Model)
	if mm.state != StateWaitingForInput {
		t.Fatalf("state = %v, want StateWaitingForInput after esc", mm.state)
	}
	if live.title != "unchanged" {
		t.Fatalf("esc changed the title to %q", live.title)
	}
	if mm.renameIdx != -1 || mm.renameID != "" {
		t.Fatalf("esc left a dangling target (idx=%d,id=%q)", mm.renameIdx, mm.renameID)
	}
}
