package ui

import "testing"

// A draft session (never connected, empty daemonSessionID) must NOT be
// reconnected when switched to. Reconnecting a draft calls session.Connect,
// which starts a fresh daemon session with no message — creating an empty
// ghost session. Regression for: create two drafts, hit enter (no-op), switch
// back to the first draft, and it gets committed empty.
func TestStepWorkspaceSession_DraftNotReconnected(t *testing.T) {
	cfg := testCfg(t.TempDir())
	a := newSessionState(cfg, nil) // draft, daemonSessionID == ""
	b := newSessionState(cfg, nil) // draft
	m := &Model{cfg: cfg, sessions: []*SessionState{a, b}, selectedSession: 1, width: 100}

	m.stepWorkspaceSession(-1) // switch back to session 0 (a draft)

	if m.selectedSession != 0 {
		t.Fatalf("expected to switch to session 0, got %d", m.selectedSession)
	}
	if a.reconnecting {
		t.Fatal("switching to a draft must not set reconnecting (would connect an empty session)")
	}
}

// A previously-live session that lost its client (has a daemonSessionID) IS
// reconnected on switch — the fix must not regress genuine reconnects.
func TestStepWorkspaceSession_LiveSessionReconnects(t *testing.T) {
	cfg := testCfg(t.TempDir())
	a := newSessionState(cfg, nil)
	a.phase = phaseLive
	a.daemonSessionID = "sess-123" // connected before, client since dropped
	b := newSessionState(cfg, nil)
	m := &Model{cfg: cfg, sessions: []*SessionState{a, b}, selectedSession: 1, width: 100}

	m.stepWorkspaceSession(-1) // switch back to session 0

	if !a.reconnecting {
		t.Fatal("a previously-connected session should reconnect on switch")
	}
}
