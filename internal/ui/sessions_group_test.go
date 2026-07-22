package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/get-vix/vix/internal/protocol"
)

// userRec builds a not-attached, user-initiated session record for the given
// directory with an activity timestamp used for recency ordering.
func userRec(id, cwd, lastReq string) protocol.SessionSummary {
	return protocol.SessionSummary{ID: id, CWD: cwd, Title: "title-" + id, LastRequestAt: lastReq}
}

// TestAbbreviatePath replaces the home prefix with "~" and labels empties.
func TestAbbreviatePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	cases := []struct{ in, want string }{
		{"", "(unknown)"},
		{"   ", "(unknown)"},
		{home, "~"},
		{filepath.Join(home, "Developer", "vix"), "~" + string(os.PathSeparator) + filepath.Join("Developer", "vix")},
		{"/opt/elsewhere", "/opt/elsewhere"},
	}
	for _, c := range cases {
		if got := abbreviatePath(c.in); got != c.want {
			t.Errorf("abbreviatePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestUserDirBlocksGrouping: user sessions are grouped by working directory with
// the current cwd first, other directories by most-recent activity (desc), and
// within a directory live sessions precede not-attached records.
func TestUserDirBlocksGrouping(t *testing.T) {
	liveWork := newSessionState(testCfg("/work"), nil) // current cwd
	liveBeta := newSessionState(testCfg("/beta"), nil) // attached other-dir session
	m := &Model{
		cwd:      "/work",
		sessions: []*SessionState{liveWork, liveBeta},
		userSessionRecords: []protocol.SessionSummary{
			userRec("rWork", "/work", "2026-01-01T00:00:00Z"),
			userRec("rAlpha", "/alpha", "2026-01-02T00:00:00Z"),
			userRec("rBeta", "/beta", "2026-01-03T00:00:00Z"),
		},
	}

	blocks := m.userDirBlocks()
	if len(blocks) != 3 {
		t.Fatalf("want 3 dir blocks, got %d", len(blocks))
	}
	// Current cwd first.
	if blocks[0].dir != "/work" {
		t.Errorf("block[0].dir = %q, want /work (current cwd first)", blocks[0].dir)
	}
	// /beta (activity 01-03) ranks above /alpha (01-02).
	if blocks[1].dir != "/beta" || blocks[2].dir != "/alpha" {
		t.Errorf("other-dir order = [%q, %q], want [/beta, /alpha]", blocks[1].dir, blocks[2].dir)
	}
	// Within /work: live session first, then its record.
	if len(blocks[0].rows) != 2 || blocks[0].rows[0].liveIdx != 0 || blocks[0].rows[1].sum == nil || blocks[0].rows[1].sum.ID != "rWork" {
		t.Errorf("/work rows = %+v, want [live#0, record rWork]", blocks[0].rows)
	}
	// Within /beta: attached live session (m.sessions[1]) first, then its record.
	if len(blocks[1].rows) != 2 || blocks[1].rows[0].liveIdx != 1 || blocks[1].rows[1].sum == nil || blocks[1].rows[1].sum.ID != "rBeta" {
		t.Errorf("/beta rows = %+v, want [live#1, record rBeta]", blocks[1].rows)
	}
}

// TestSessionRowTargetsIncludesUserRecords: the flat selection order lists the
// User-initiated rows (grouped by dir) before the Vix-initiated rows, so the
// selection index space covers cross-directory user records.
func TestSessionRowTargetsIncludesUserRecords(t *testing.T) {
	liveWork := newSessionState(testCfg("/work"), nil)
	vixRec := protocol.SessionSummary{ID: "vixRun", CWD: "/job", Origin: "vix", StartedAt: "2026-01-05T00:00:00Z"}
	m := &Model{
		cwd:      "/work",
		sessions: []*SessionState{liveWork},
		userSessionRecords: []protocol.SessionSummary{
			userRec("rWork", "/work", "2026-01-01T00:00:00Z"),
			userRec("rAlpha", "/alpha", "2026-01-02T00:00:00Z"),
		},
		vixSessions: []protocol.SessionSummary{vixRec},
	}

	rows := m.sessionRowTargets()
	if len(rows) != 4 {
		t.Fatalf("want 4 rows (1 live + 2 user records + 1 vix), got %d", len(rows))
	}
	// User section: live /work, record rWork, record rAlpha.
	if rows[0].sum != nil || rows[0].liveIdx != 0 {
		t.Errorf("row[0] should be the live /work session, got %+v", rows[0])
	}
	if rows[1].sum == nil || rows[1].sum.ID != "rWork" {
		t.Errorf("row[1] should be record rWork, got %+v", rows[1])
	}
	if rows[2].sum == nil || rows[2].sum.ID != "rAlpha" {
		t.Errorf("row[2] should be record rAlpha, got %+v", rows[2])
	}
	// Vix section last.
	if rows[3].sum == nil || rows[3].sum.ID != "vixRun" {
		t.Errorf("row[3] should be the vix record, got %+v", rows[3])
	}
}

// TestUserDirBlocksDedupsLiveRecords: a record that is already live in this
// window (attached but the list hasn't refreshed) is not shown twice.
func TestUserDirBlocksDedupsLiveRecords(t *testing.T) {
	live := newSessionState(testCfg("/work"), nil)
	live.daemonSessionID = "dup-id"
	m := &Model{
		cwd:      "/work",
		sessions: []*SessionState{live},
		userSessionRecords: []protocol.SessionSummary{
			{ID: "dup-id", CWD: "/work", Title: "dup", LastRequestAt: "2026-01-01T00:00:00Z"},
			userRec("keep", "/work", "2026-01-02T00:00:00Z"),
		},
	}
	blocks := m.userDirBlocks()
	if len(blocks) != 1 {
		t.Fatalf("want 1 dir block, got %d", len(blocks))
	}
	// Live row + the non-duplicate record only (the "dup-id" record is dropped).
	if len(blocks[0].rows) != 2 {
		t.Fatalf("want 2 rows (live + keep), got %d: %+v", len(blocks[0].rows), blocks[0].rows)
	}
	for _, r := range blocks[0].rows {
		if r.sum != nil && r.sum.ID == "dup-id" {
			t.Error("record duplicating a live session should be dropped")
		}
	}
}

// TestRenderSessionsViewGroupsByDir: the User-initiated group renders a path
// subtitle for every directory (always shown) and the rows under each.
func TestRenderSessionsViewGroupsByDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	workDir := filepath.Join(home, "work")
	groups := []userDirGroupView{
		{dir: workDir, rows: []userRowView{{sum: protocol.SessionSummary{ID: "s1abc", Title: "Alpha title", LastRequestAt: "2026-01-01T00:00:00Z"}}}},
		{dir: "/opt/proj", rows: []userRowView{{sum: protocol.SessionSummary{ID: "s2xyz", Title: "Beta title", LastRequestAt: "2026-01-01T00:00:00Z"}}}},
	}
	out := renderSessionsView(groups, nil, 120, 40, NewStyles(true), 0, "")

	for _, want := range []string{
		"User-initiated",
		"~" + string(os.PathSeparator) + "work", // home-abbreviated subtitle, always shown
		"/opt/proj",                             // second directory subtitle
		"Alpha title", "Beta title",
		"s1abc", "s2xyz",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSessionsView output missing %q\n---\n%s", want, out)
		}
	}
}
