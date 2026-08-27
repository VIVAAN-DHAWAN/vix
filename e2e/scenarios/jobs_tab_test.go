package scenarios

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/get-vix/vix/e2e/harness"
)

// This file exercises the Jobs & Triggers tab (F5): it lists scheduled jobs and
// lifecycle hooks in two groups, toggles a row's enabled state with space
// (persisted to the spec file), and confirms the F-key remap (MCP is now F4,
// Jobs & Triggers F5, Settings F6). Jobs must be enabled (VIX_DISABLE_JOBS=0)
// for the scheduler to surface them.

func jobsTabMeta(desc string) harness.Meta {
	return harness.Meta{
		Category:    "ui",
		Subcategory: "ui.jobs",
		Description: desc,
		Wire:        harness.WireMessages,
	}
}

// A far-future one-shot job: enabled and listed, but never fires during the
// test, so its enabled state stays stable until we toggle it.
const jobsTabJobSpec = `{
  "id": "e2e-tabjob",
  "name": "E2E Tab Job",
  "enabled": true,
  "trigger": {"type": "at", "time": "2999-01-01T00:00:00Z"},
  "prompt": "Say hello.",
  "cwd": "{{WORKDIR}}",
  "created_by": "user"
}`

const jobsTabHookSpec = `{
  "id": "e2e-tabhook",
  "name": "E2E Tab Hook",
  "enabled": true,
  "trigger": {"event": "PreToolUse", "matcher": "bash"},
  "command": "true"
}`

// TestJobsTabListsAndToggles verifies F5 opens the Jobs & Triggers tab with both
// groups, the header (docs link + prompt example), the enabled checkboxes, and
// that space toggles the selected job off — persisted to its job.json.
func TestJobsTabListsAndToggles(t *testing.T) {
	h := harness.Start(t, jobsTabMeta("F5 lists jobs & triggers; space toggles a job off, persisted to job.json"),
		harness.WithEnv("VIX_DISABLE_JOBS", "0"),
		harness.WithHomeFile(".vix/jobs/e2e-tabjob/job.json", jobsTabJobSpec),
		harness.WithHomeFile(".vix/hooks/e2e-tabhook/hook.json", jobsTabHookSpec),
	)

	h.UI.WaitStable(500 * time.Millisecond)
	h.UI.Key("f5")
	h.UI.WaitFor("E2E Tab Job")
	h.UI.WaitStable(300 * time.Millisecond)
	h.UI.Shot("jobs-tab")

	for _, want := range []string{
		"Jobs & Triggers [F5]", "Settings [F6]", // tab bar after remap
		"Jobs", "Triggers", // group headers
		"E2E Tab Job", "E2E Tab Hook", // a job and a hook row
		"getvix.dev/docs#guide-jobs", // header docs link
		"Every weekday at 9am",       // header prompt example
		"PreToolUse",                 // hook event column
		"[✓]",                        // enabled checkbox
	} {
		if !h.UI.Contains(want) {
			t.Fatalf("Jobs & Triggers tab missing %q; screen:\n%s", want, h.UI.Snapshot())
		}
	}

	// The cursor starts on the first job; space toggles it off.
	h.UI.Key("space")
	jobPath := h.HomePath(".vix/jobs/e2e-tabjob/job.json")
	readJob := func() string {
		b, _ := os.ReadFile(jobPath)
		return string(b)
	}
	if !pollUntil(8*time.Second, func() bool {
		return strings.Contains(readJob(), `"enabled": false`)
	}) {
		t.Fatalf("job.json at %s not flipped to disabled; got:\n%s", jobPath, readJob())
	}
	// The list refreshes live and shows the disabled checkbox.
	if !pollUntil(5*time.Second, func() bool { return h.UI.Contains("[ ]") }) {
		t.Fatalf("disabled checkbox not shown after toggle; screen:\n%s", h.UI.Snapshot())
	}
	h.UI.Shot("jobs-tab-toggled")
}

// TestJobsTabFKeyRemap guards the tab remap: F5 opens Jobs & Triggers and F6
// opens Settings (previously F4/F5).
func TestJobsTabFKeyRemap(t *testing.T) {
	h := harness.Start(t, jobsTabMeta("F5 opens Jobs & Triggers and F6 opens Settings after the remap"),
		harness.WithEnv("VIX_DISABLE_JOBS", "0"),
	)

	h.UI.WaitStable(500 * time.Millisecond)
	h.UI.Key("f5")
	// Wait on a Jobs-body-only string: the tab bar always contains "Jobs" and
	// "Triggers" ("Jobs & Triggers [F5]"), so waiting on those would
	// false-positive before the tab body actually paints.
	h.UI.WaitFor("getvix.dev/docs#guide-jobs")

	h.UI.Key("f6")
	h.UI.WaitFor("Auto-compaction")
	h.UI.Shot("settings-f6")
}

// badgeJobSpec is a far-future one-shot job: enabled and listed, but it never
// fires during the test, so the seeded run history (state.json) stays intact.
const badgeJobSpec = `{
  "id": "e2e-badgejob",
  "name": "E2E Badge Job",
  "enabled": true,
  "trigger": {"type": "at", "time": "2999-01-01T00:00:00Z"},
  "prompt": "Say hello.",
  "cwd": "{{WORKDIR}}",
  "created_by": "user"
}`

// badgeJobState seeds a recent-run history of 3 runs, 2 of which failed (one
// error + one timeout), so the Jobs tab must render the "2/3" error badge.
const badgeJobState = `{
  "recent_runs": [
    {"at": "2024-01-01T00:00:00Z", "status": "ok"},
    {"at": "2024-01-02T00:00:00Z", "status": "error"},
    {"at": "2024-01-03T00:00:00Z", "status": "timeout"}
  ]
}`

// TestJobsTabErrorBadge verifies the Jobs & Triggers tab surfaces a job's recent
// run health as an "<errors>/<runs>" badge in the Errors column, counting error
// and timeout runs (not ok) across the capped history.
func TestJobsTabErrorBadge(t *testing.T) {
	h := harness.Start(t, jobsTabMeta("F5 shows the per-job recent-run error badge (N/M) in the Errors column"),
		harness.WithEnv("VIX_DISABLE_JOBS", "0"),
		harness.WithHomeFile(".vix/jobs/e2e-badgejob/job.json", badgeJobSpec),
		harness.WithHomeFile(".vix/jobs/e2e-badgejob/state.json", badgeJobState),
	)

	h.UI.WaitStable(500 * time.Millisecond)
	h.UI.Key("f5")
	h.UI.WaitFor("E2E Badge Job")
	h.UI.WaitStable(300 * time.Millisecond)

	for _, want := range []string{
		"Errors", // the new column header
		"2/3",    // 2 failed (error + timeout) out of the last 3 runs
	} {
		if !h.UI.Contains(want) {
			t.Fatalf("Jobs & Triggers tab missing %q; screen:\n%s", want, h.UI.Snapshot())
		}
	}
	h.UI.Shot("jobs-tab-error-badge")
}
