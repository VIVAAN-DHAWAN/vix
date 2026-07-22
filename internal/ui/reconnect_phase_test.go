package ui

import (
	"testing"

	"github.com/get-vix/vix/internal/daemon"
)

// A duplicated/forked session is created as a draft and connected through
// connectFork → reconnectSuccessMsg. That handler must promote it to phaseLive:
// otherwise its first follow-up message falls into the draft-commit branch and
// connectDraft a fresh, empty daemon session — throwing away the fork-seeded
// history (the "duplicate-of-a-duplicate starts empty" regression).
func TestReconnectSuccess_PromotesDraftToLive(t *testing.T) {
	cfg := testCfg(t.TempDir())
	sess := newSessionState(cfg, nil) // phaseDraft, daemonSessionID ""
	sess.reconnecting = true
	sess.chatMessages = []ChatMessage{{}} // seeded copy (non-empty)
	m := &Model{cfg: cfg, sessions: []*SessionState{sess}, selectedSession: 0, width: 100}

	client := daemon.NewSessionClient(cfg.SocketPath)
	m.updateInner(reconnectSuccessMsg{daemonSessionID: "", client: client})

	if sess.phase != phaseLive {
		t.Fatalf("reconnected session must be promoted to phaseLive, got %v", sess.phase)
	}
	if sess.reconnecting {
		t.Fatal("reconnected session must clear reconnecting")
	}
	if sess.client != client {
		t.Fatal("reconnected session must adopt the new client")
	}
}
