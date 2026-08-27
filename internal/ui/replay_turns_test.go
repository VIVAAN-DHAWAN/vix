package ui

import (
	"testing"

	"github.com/get-vix/vix/internal/protocol"
)

// Turn separators are UI-only markers a live run appends at each turn end; they
// are not persisted nor re-sent in event.replay. buildReplayChatMessages must
// reconstruct one per completed turn (an assistant message with no tool_use
// block) so a thread restored on relaunch is forkable/trimmable/duplicable —
// otherwise turnSeparatorInfos finds nothing and duplicate is refused with
// "no completed turns yet".
func TestBuildReplayChatMessages_ReconstructsTurnSeparators(t *testing.T) {
	m := &Model{
		width:      120,
		styles:     NewStyles(true),
		mdRenderer: NewMarkdownRenderer(116, true, NewStyles(true).CodeBoxBorderStyle),
	}

	rep := protocol.EventReplay{
		Model: "anthropic/claude-sonnet-4-6",
		Messages: []protocol.ReplayMessage{
			// Turn 1: a tool-using round (no separator) followed by the final
			// text-only assistant message that ends the turn.
			{Role: "user", Blocks: []protocol.ReplayBlock{{Kind: "text", Text: "do a thing"}}},
			{Role: "assistant", Blocks: []protocol.ReplayBlock{{Kind: "tool_use", ToolID: "t1", ToolName: "bash", Input: map[string]any{"command": "ls"}}}},
			{Role: "user", Blocks: []protocol.ReplayBlock{{Kind: "tool_result", ToolID: "t1", Output: "file.txt"}}},
			{Role: "assistant", Blocks: []protocol.ReplayBlock{{Kind: "text", Text: "done with turn one"}}},
			// Turn 2: a plain text turn.
			{Role: "user", Blocks: []protocol.ReplayBlock{{Kind: "text", Text: "another"}}},
			{Role: "assistant", Blocks: []protocol.ReplayBlock{{Kind: "text", Text: "done with turn two"}}},
		},
	}

	msgs := m.buildReplayChatMessages(rep)

	// Two completed turns -> two separators, numbered 1 and 2.
	var seps []ChatMessage
	for _, c := range msgs {
		if c.Type == MsgSystem && c.TurnModel != "" {
			seps = append(seps, c)
		}
	}
	if len(seps) != 2 {
		t.Fatalf("want 2 reconstructed turn separators, got %d", len(seps))
	}
	if seps[0].TurnNum != 1 || seps[1].TurnNum != 2 {
		t.Fatalf("separator turn numbers = %d,%d; want 1,2", seps[0].TurnNum, seps[1].TurnNum)
	}
	if seps[0].TurnModel != rep.Model {
		t.Fatalf("separator model = %q, want %q", seps[0].TurnModel, rep.Model)
	}

	// The public accessor the duplicate/fork/trim paths use must now see them.
	if got := turnSeparatorInfos(msgs, m.styles, m.mdRenderer.width); len(got) != 2 {
		t.Fatalf("turnSeparatorInfos found %d separators on a restored transcript, want 2", len(got))
	}
}

// A trailing tool-using assistant message (an interrupted, still-open turn) must
// NOT get a separator — only completed turns are forkable, mirroring the live
// path and the daemon's rebuildTurnSnapshots.
func TestBuildReplayChatMessages_NoSeparatorForOpenTurn(t *testing.T) {
	m := &Model{
		width:      120,
		styles:     NewStyles(true),
		mdRenderer: NewMarkdownRenderer(116, true, NewStyles(true).CodeBoxBorderStyle),
	}

	rep := protocol.EventReplay{
		Model: "anthropic/claude-sonnet-4-6",
		Messages: []protocol.ReplayMessage{
			{Role: "user", Blocks: []protocol.ReplayBlock{{Kind: "text", Text: "go"}}},
			// Ends on a tool_use with no closing text: the turn never completed.
			{Role: "assistant", Blocks: []protocol.ReplayBlock{{Kind: "tool_use", ToolID: "t1", ToolName: "bash", Input: map[string]any{"command": "ls"}}}},
		},
	}

	msgs := m.buildReplayChatMessages(rep)
	if got := turnSeparatorInfos(msgs, m.styles, m.mdRenderer.width); len(got) != 0 {
		t.Fatalf("an open (tool_use) turn must not produce a separator, got %d", len(got))
	}
}
