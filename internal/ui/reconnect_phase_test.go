package ui

import (
	"testing"

	"github.com/get-vix/vix/internal/daemon"
)

// A duplicated/forked thread is created as a draft and connected through
// connectFork → reconnectSuccessMsg. That handler must promote it to phaseLive:
// otherwise its first follow-up message falls into the draft-commit branch and
// connectDraft a fresh, empty daemon thread — throwing away the fork-seeded
// history (the "duplicate-of-a-duplicate starts empty" regression).
func TestReconnectSuccess_PromotesDraftToLive(t *testing.T) {
	cfg := testCfg(t.TempDir())
	sess := newThreadState(cfg, nil) // phaseDraft, daemonThreadID ""
	sess.reconnecting = true
	sess.chatMessages = []ChatMessage{{}} // seeded copy (non-empty)
	m := &Model{cfg: cfg, threads: []*ThreadState{sess}, selectedThread: 0, width: 100}

	client := daemon.NewThreadClient(cfg.SocketPath)
	m.updateInner(reconnectSuccessMsg{daemonThreadID: "", client: client})

	if sess.phase != phaseLive {
		t.Fatalf("reconnected thread must be promoted to phaseLive, got %v", sess.phase)
	}
	if sess.reconnecting {
		t.Fatal("reconnected thread must clear reconnecting")
	}
	if sess.client != client {
		t.Fatal("reconnected thread must adopt the new client")
	}
}

// A duplicate/fork tab has no daemonThreadID yet, so connectFork correlates the
// success back to it by clientKey. When another draft (unsent Ctrl+T tab,
// lingering launch welcome) already sits earlier in the list with an equally
// empty daemonThreadID, matching by daemon id would adopt the fork client onto
// that stray draft — the duplicate tab would keep showing the copied messages
// with no live daemon history behind them (the "appears duplicated but empty"
// bug). The clientKey must route the client to the actual fork tab.
func TestReconnectSuccess_ForkMatchesByClientKeyNotStrayDraft(t *testing.T) {
	cfg := testCfg(t.TempDir())

	// A stray, never-connected draft sits first: empty daemonThreadID.
	stray := newThreadState(cfg, nil)

	// The duplicate tab: also empty daemonThreadID, seeded copy, connecting.
	dup := newThreadState(cfg, nil)
	dup.reconnecting = true
	dup.chatMessages = []ChatMessage{{}}

	m := &Model{cfg: cfg, threads: []*ThreadState{stray, dup}, selectedThread: 1, width: 100}

	client := daemon.NewThreadClient(cfg.SocketPath)
	// connectFork stamps the fork tab's clientKey (daemonThreadID stays "").
	m.updateInner(reconnectSuccessMsg{daemonThreadID: "", clientKey: dup.clientKey, client: client})

	if dup.client != client {
		t.Fatal("the duplicate tab must adopt the fork client")
	}
	if dup.phase != phaseLive {
		t.Fatalf("the duplicate tab must be promoted to phaseLive, got %v", dup.phase)
	}
	if dup.reconnecting {
		t.Fatal("the duplicate tab must clear reconnecting")
	}
	if stray.client == client {
		t.Fatal("the fork client must NOT be adopted by the stray draft")
	}
	if stray.client != nil {
		t.Fatal("the stray draft must remain a client-less draft")
	}
	if stray.phase != phaseDraft {
		t.Fatalf("the stray draft must stay phaseDraft, got %v", stray.phase)
	}
}
