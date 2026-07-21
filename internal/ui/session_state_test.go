package ui

import (
	"testing"

	"github.com/get-vix/vix/internal/config"
)

func testCfg(dir string) *config.Config {
	return &config.Config{
		CWD:   dir,
		Model: "anthropic/claude-sonnet-4-6",
		Paths: config.NewVixPaths("", dir, dir),
	}
}

func TestNewSessionState_NilClientIsDraft(t *testing.T) {
	dir := t.TempDir()
	s := newSessionState(testCfg(dir), nil)
	if s.phase != phaseDraft {
		t.Errorf("nil client should yield phaseDraft, got %v", s.phase)
	}
	if s.client != nil {
		t.Error("draft session should have a nil client")
	}
	if s.workDir != dir {
		t.Errorf("workDir: want %q, got %q", dir, s.workDir)
	}
	if s.clientKey == "" {
		t.Error("clientKey should be set")
	}
	if s.daemonSessionID != "" {
		t.Errorf("draft daemonSessionID should be empty, got %q", s.daemonSessionID)
	}
}

func TestNewSessionState_ClientKeysUnique(t *testing.T) {
	dir := t.TempDir()
	a := newSessionState(testCfg(dir), nil)
	b := newSessionState(testCfg(dir), nil)
	if a.clientKey == b.clientKey {
		t.Errorf("two drafts share a clientKey: %q", a.clientKey)
	}
}

func TestFindSessionByClientKey(t *testing.T) {
	dir := t.TempDir()
	a := newSessionState(testCfg(dir), nil)
	b := newSessionState(testCfg(dir), nil)
	m := Model{sessions: []*SessionState{a, b}}

	idx, got := m.findSessionByClientKey(b.clientKey)
	if idx != 1 || got != b {
		t.Errorf("findSessionByClientKey returned idx=%d got=%p, want 1/%p", idx, got, b)
	}
	if _, got := m.findSessionByClientKey("nope"); got != nil {
		t.Error("unknown clientKey should return nil")
	}
}

func TestPickCWD(t *testing.T) {
	if got := pickCWD("/a", "/b"); got != "/a" {
		t.Errorf("pickCWD non-blank primary: want /a, got %q", got)
	}
	if got := pickCWD("  ", "/b"); got != "/b" {
		t.Errorf("pickCWD blank primary: want fallback /b, got %q", got)
	}
}
