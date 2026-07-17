package daemon

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/get-vix/vix/internal/protocol"
	"github.com/get-vix/vix/internal/protocol/protoschema"
)

// TestProtocolConformanceLiveSocket is the protocol end-to-end guard: it drives
// a real Server over a real Unix socket with a real client handshake, then
// validates every event the daemon actually emits against the generated schema
// (internal/protocol/schema/vix-protocol.schema.json) — the exact contract the
// native Swift client (apps/vix-mac) decodes.
//
// The tmux e2e suite is screen-oriented and cannot inspect raw events, so this
// lives in the daemon package where a real in-process Server + socket are cheap.
// It exercises the live surface (session lifecycle, init, and an input turn);
// the full per-type surface is covered by protoschema.TestRoundTrip.
func TestProtocolConformanceLiveSocket(t *testing.T) {
	// Empty daemon version disables the version gate, so an empty client
	// version is accepted (in-process embedding convention).
	srv := newInstanceTestServer(t)
	_, cancel := serve(t, srv)
	defer cancel()

	conn, err := net.Dial("unix", srv.sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	send := func(cmd protocol.SessionCommand) {
		t.Helper()
		payload, _ := json.Marshal(cmd)
		payload = append(payload, '\n')
		if _, err := conn.Write(payload); err != nil {
			t.Fatalf("write %s: %v", cmd.Type, err)
		}
	}

	dec := json.NewDecoder(conn)
	readEvent := func() (protocol.SessionEvent, bool) {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var ev protocol.SessionEvent
		if err := dec.Decode(&ev); err != nil {
			return protocol.SessionEvent{}, false
		}
		return ev, true
	}

	validated := map[string]bool{}
	check := func(ev protocol.SessionEvent) {
		t.Helper()
		if err := protoschema.ValidateEvent(ev.Type, ev.Data); err != nil {
			t.Errorf("event %q does not conform to schema: %v", ev.Type, err)
		}
		validated[ev.Type] = true
	}

	// Start the session and drain to session_started, validating each event.
	startData, _ := json.Marshal(protocol.SessionStartData{CWD: t.TempDir()})
	send(protocol.SessionCommand{Type: "session.start", Data: startData})

	gotStarted := false
	for i := 0; i < 50 && !gotStarted; i++ {
		ev, ok := readEvent()
		if !ok {
			break
		}
		check(ev)
		if ev.Type == "event.session_started" {
			gotStarted = true
		}
	}
	if !gotStarted {
		t.Fatal("did not receive event.session_started")
	}

	// Drive an input turn. Without configured credentials the daemon emits an
	// error notice followed by agent_done — both real events over the wire.
	inputData, _ := json.Marshal(protocol.SessionInputData{Text: "hello"})
	send(protocol.SessionCommand{Type: "session.input", Data: inputData})

	gotDone := false
	for i := 0; i < 200 && !gotDone; i++ {
		ev, ok := readEvent()
		if !ok {
			break
		}
		check(ev)
		if ev.Type == "event.agent_done" {
			gotDone = true
		}
	}
	if !gotDone {
		t.Fatal("did not receive event.agent_done after input")
	}

	// Sanity: we actually saw and validated the core lifecycle events.
	for _, want := range []string{"event.session_started", "event.agent_done"} {
		if !validated[want] {
			t.Errorf("expected to validate %q but never saw it", want)
		}
	}
}
