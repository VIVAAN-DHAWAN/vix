package ui

import (
	"strings"
	"testing"
)

func TestRenderWelcomeInline_DraftShowsWorkDirAndHint(t *testing.T) {
	s := NewStyles(true)
	out := renderWelcomeInline(120, 40, s, "/tmp/project", true)
	if !strings.Contains(out, "/tmp/project") {
		t.Error("welcome should show the working directory")
	}
	if !strings.Contains(out, "Ctrl+O") {
		t.Error("draft welcome should advertise Ctrl+O to change directory")
	}
}

func TestRenderWelcomeInline_LiveOmitsChangeHint(t *testing.T) {
	s := NewStyles(true)
	out := renderWelcomeInline(120, 40, s, "/tmp/project", false)
	if !strings.Contains(out, "/tmp/project") {
		t.Error("welcome should still show the working directory when not a draft")
	}
	if strings.Contains(out, "your first message starts the session here") {
		t.Error("non-draft welcome should not show the draft start note")
	}
}
