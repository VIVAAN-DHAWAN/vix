package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/get-vix/vix/internal/config"
	"github.com/get-vix/vix/internal/daemon/llm"
	"github.com/get-vix/vix/internal/protocol"
)

// benchQuiet silences the stdlib logger for the duration of a benchmark so log
// lines neither pollute the benchstat output nor skew timings.
func benchQuiet(tb testing.TB) {
	tb.Helper()
	prev := log.Writer()
	log.SetOutput(io.Discard)
	tb.Cleanup(func() { log.SetOutput(prev) })
}

// Performance benchmarks for daemon hot paths. The model is stubbed with
// fakeCompactionLLM (see compaction_test.go); disk work runs against real,
// isolated temp dirs (b.TempDir) and access-stats against an in-memory SQLite.
// These share the daemon package's white-box test scope, so they reuse
// sampleRecord / fakeCompactionLLM directly.

func benchRecord(id string) threadRecord {
	r := sampleRecord()
	r.ID = id
	return r
}

// BenchmarkThreadStoreSaveLoad measures a save + full-list round trip against a
// store already holding N records, so the list cost's scaling is visible.
func BenchmarkThreadStoreSaveLoad(b *testing.B) {
	for _, n := range []int{1, 100, 10000} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			paths := config.NewVixPaths(b.TempDir(), "", "/work")
			for i := 0; i < n; i++ {
				if err := saveThreadRecord(paths, benchRecord(fmt.Sprintf("seed-%06d", i))); err != nil {
					b.Fatal(err)
				}
			}
			hot := benchRecord("bench-hot")
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := saveThreadRecord(paths, hot); err != nil {
					b.Fatal(err)
				}
				if got := listOpenThreadRecords(paths); len(got) != n+1 {
					b.Fatalf("listed %d records, want %d", len(got), n+1)
				}
			}
		})
	}
}

// BenchmarkAccessStatsLog measures a single access-log insert against an
// in-memory SQLite DB (the write-mostly hot path).
func BenchmarkAccessStatsLog(b *testing.B) {
	db, err := initAccessStatsDB(":memory:")
	if err != nil {
		b.Fatalf("initAccessStatsDB: %v", err)
	}
	defer db.Close()
	params := map[string]any{"path": "src/file.go"}
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		path := fmt.Sprintf("src/file_%d.go", i%256)
		if err := logFileAccess(db, "thread-1", path, "read_file", "go", params); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

// BenchmarkAccessStatsTopFiles measures the top-N aggregation query over a
// pre-seeded table.
func BenchmarkAccessStatsTopFiles(b *testing.B) {
	db, err := initAccessStatsDB(":memory:")
	if err != nil {
		b.Fatalf("initAccessStatsDB: %v", err)
	}
	defer db.Close()
	for i := 0; i < 10000; i++ {
		if err := logFileAccess(db, "t", fmt.Sprintf("src/f_%d.go", i%500), "read_file", "go", nil); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := getTopAccessedFiles(db, 20); err != nil {
			b.Fatal(err)
		}
	}
}

// newAgentBenchThread builds a Thread wired with a stubbed model, a drained
// event channel, real temp-dir persistence, and a modest conversation history.
func newAgentBenchThread(tb testing.TB) (*Thread, func()) {
	tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan protocol.ThreadEvent, 256)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-events:
			case <-done:
				return
			}
		}
	}()

	msgs := make([]llm.MessageParam, 0, 12)
	for i := 0; i < 6; i++ {
		msgs = append(msgs,
			llm.NewUserMessage(llm.NewTextBlock(strings.Repeat("question ", 40))),
			llm.NewAssistantMessage(llm.NewTextBlock(strings.Repeat("answer ", 60))),
		)
	}

	s := &Thread{
		ctx:       ctx,
		eventChan: events,
		llm:       &fakeCompactionLLM{summary: "ok"},
		model:     "anthropic/claude-opus-4-8",
		paths:     config.NewVixPaths(tb.TempDir(), "", "/work"),
		id:        "bench-agent-turn",
		cwd:       "/work",
		messages:  msgs,
		startTime: time.Now(),
	}
	return s, func() { cancel(); close(done) }
}

// agentTurnOnce exercises the turn scaffolding with a stubbed model
// (streamWithRetry → build record → persist to disk). Tool dispatch is out of
// scope here — it requires the sandbox harness — so this measures the
// request/decode loop plus record-build and the real disk persist.
func agentTurnOnce(tb testing.TB, s *Thread) {
	tb.Helper()
	msg, _, err := s.streamWithRetry(nil, nil, nil)
	if err != nil {
		tb.Fatalf("streamWithRetry: %v", err)
	}
	if msg == nil {
		tb.Fatal("streamWithRetry returned nil message")
	}
	s.persist()
}

func BenchmarkAgentTurn(b *testing.B) {
	benchQuiet(b)
	s, cleanup := newAgentBenchThread(b)
	defer cleanup()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		agentTurnOnce(b, s)
	}
}

// TestPerfBenchmarks_Smoke runs each daemon benchmark body once under `go test`
// so `make test` guards against the benchmarks breaking.
func TestPerfBenchmarks_Smoke(t *testing.T) {
	paths := config.NewVixPaths(t.TempDir(), "", "/work")
	if err := saveThreadRecord(paths, benchRecord("smoke")); err != nil {
		t.Fatal(err)
	}
	if got := listOpenThreadRecords(paths); len(got) != 1 {
		t.Fatalf("listOpenThreadRecords = %d, want 1", len(got))
	}

	db, err := initAccessStatsDB(":memory:")
	if err != nil {
		t.Fatalf("initAccessStatsDB: %v", err)
	}
	defer db.Close()
	if err := logFileAccess(db, "t", "a.go", "read_file", "go", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := getTopAccessedFiles(db, 5); err != nil {
		t.Fatal(err)
	}

	s, cleanup := newAgentBenchThread(t)
	defer cleanup()
	agentTurnOnce(t, s)
}
