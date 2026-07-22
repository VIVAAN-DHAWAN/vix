package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/get-vix/vix/internal/config"
	"github.com/get-vix/vix/internal/protocol"
)

// runCmdMsgs executes a tea.Cmd, flattening any tea.BatchMsg, and returns the
// concrete messages the command(s) produced. Commands that hit the daemon fail
// fast against the empty socket path and return their zero-value result message,
// which is enough to identify which command was dispatched.
func runCmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch m := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, runCmdMsgs(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func hasMsgType[T any](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

func newInstanceEventTestModel() Model {
	return Model{
		cfg:        &config.Config{},
		socketPath: "",
		cwd:        "/tmp",
		authToken:  "",
	}
}

// TestInstanceEvent_SessionsChanged: a sessions_changed control event dispatches
// both a vix-sessions refresh and a recent-dirs refresh.
func TestInstanceEvent_SessionsChanged(t *testing.T) {
	m := newInstanceEventTestModel()
	_, cmd := m.updateInner(instanceEventMsg{event: protocol.SessionEvent{Type: "event.sessions_changed"}})
	msgs := runCmdMsgs(cmd)
	if !hasMsgType[vixSessionsMsg](msgs) {
		t.Error("sessions_changed should dispatch fetchVixSessions (vixSessionsMsg)")
	}
	if !hasMsgType[recentDirsMsg](msgs) {
		t.Error("sessions_changed should dispatch fetchRecentDirs (recentDirsMsg)")
	}
}

// TestInstanceEvent_JobsChanged: a jobs_changed control event refreshes the Jobs
// & Triggers tab only while that tab is active.
func TestInstanceEvent_JobsChanged(t *testing.T) {
	// Active Jobs tab → refresh dispatched.
	m := newInstanceEventTestModel()
	m.activeTab = TabKindJobs
	_, cmd := m.updateInner(instanceEventMsg{event: protocol.SessionEvent{Type: "event.jobs_changed"}})
	if !hasMsgType[jobsListMsg](runCmdMsgs(cmd)) {
		t.Error("jobs_changed on the Jobs tab should dispatch fetchJobsAndHooks (jobsListMsg)")
	}

	// Any other tab → no fetch.
	m2 := newInstanceEventTestModel()
	m2.activeTab = TabKindChat
	_, cmd2 := m2.updateInner(instanceEventMsg{event: protocol.SessionEvent{Type: "event.jobs_changed"}})
	if hasMsgType[jobsListMsg](runCmdMsgs(cmd2)) {
		t.Error("jobs_changed off the Jobs tab should not fetch")
	}
}

// TestInstanceEvent_Quit: a quit control event asks Bubble Tea to quit.
func TestInstanceEvent_Quit(t *testing.T) {
	m := newInstanceEventTestModel()
	_, cmd := m.updateInner(instanceEventMsg{event: protocol.SessionEvent{Type: "event.quit"}})
	if !hasMsgType[tea.QuitMsg](runCmdMsgs(cmd)) {
		t.Error("quit control event should dispatch tea.Quit (tea.QuitMsg)")
	}
}
