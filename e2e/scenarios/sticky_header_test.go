package scenarios

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/get-vix/vix/e2e/harness"
)

// longReply builds an assistant reply as a fenced code block so glamour renders
// each line on its own row (no paragraph reflow). It is long enough to overflow
// the viewport — pushing the turn's user prompt off screen — and ends in a
// unique marker the test can wait for.
func longReply(marker string) string {
	var b strings.Builder
	b.WriteString("```\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "%s body line %02d\n", marker, i)
	}
	b.WriteString(marker + "_REPLY_END\n")
	b.WriteString("```")
	return b.String()
}

// TestStickyTurnHeader proves the chat viewport pins a one-line sticky header
// showing the user prompt of the turn at the top of the view while scrolled,
// and hides it at the bottom. Two turns each get a viewport-overflowing reply,
// so each prompt scrolls off screen; the only way a prompt reappears is via the
// sticky header.
func TestStickyTurnHeader(t *testing.T) {
	h := harness.Start(t, harness.Meta{
		Category:    "ui",
		Subcategory: "ui.sticky_header",
		Description: "scrolling pins a sticky header with the current turn's user prompt",
		Wire:        harness.WireMessages,
	}, harness.WithTermSize(120, 24))

	h.UI.WaitStable(400 * time.Millisecond)
	h.UI.Shot("initial")

	// Turn 1.
	h.Mock.Enqueue(harness.Text(longReply("ALPHA")))
	h.UI.Type("ALPHA_PROMPT_MARKER")
	h.UI.Enter()
	h.UI.WaitFor("ALPHA_REPLY_END")

	// Turn 2.
	h.Mock.Enqueue(harness.Text(longReply("BETA")))
	h.UI.Type("BETA_PROMPT_MARKER")
	h.UI.Enter()
	h.UI.WaitFor("BETA_REPLY_END")
	h.UI.WaitStable(400 * time.Millisecond)
	h.UI.Shot("bottom-no-header")

	// At the bottom both prompts have scrolled off; no sticky header shows them.
	if h.UI.Contains("BETA_PROMPT_MARKER") {
		t.Fatalf("at bottom the sticky header should be hidden, but BETA_PROMPT_MARKER is visible; screen:\n%s", h.UI.Snapshot())
	}
	if h.UI.Contains("ALPHA_PROMPT_MARKER") {
		t.Fatalf("at bottom turn-1 prompt should be off screen; screen:\n%s", h.UI.Snapshot())
	}

	// Focus the chat viewport and scroll up a little. We move only ~3 rows, far
	// less than the 45-line reply, so the turn-2 prompt cannot enter the body —
	// its reappearance can only be the sticky header.
	h.UI.Key("tab")
	h.UI.Key("up")
	h.UI.WaitStable(300 * time.Millisecond)
	h.UI.Shot("scrolled-turn2-header")

	if !h.UI.Contains("BETA_PROMPT_MARKER") {
		t.Fatalf("after scrolling, the sticky header should show the turn-2 prompt; screen:\n%s", h.UI.Snapshot())
	}
	if !h.UI.Contains("BETA body line") {
		t.Fatalf("expected turn-2 reply body still visible under the header; screen:\n%s", h.UI.Snapshot())
	}

	// Scroll to the very top: the header now tracks turn 1's prompt.
	h.UI.Key("home")
	h.UI.WaitStable(300 * time.Millisecond)
	h.UI.Shot("scrolled-top-turn1-header")
	if !h.UI.Contains("ALPHA_PROMPT_MARKER") {
		t.Fatalf("at the top the sticky header should show the turn-1 prompt; screen:\n%s", h.UI.Snapshot())
	}

	// Back to the bottom: header gone again.
	h.UI.Key("end")
	h.UI.WaitStable(300 * time.Millisecond)
	h.UI.Shot("bottom-again-no-header")
	if h.UI.Contains("ALPHA_PROMPT_MARKER") || h.UI.Contains("BETA_PROMPT_MARKER") {
		t.Fatalf("returning to the bottom should hide the sticky header; screen:\n%s", h.UI.Snapshot())
	}
}
