package daemon

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/get-vix/vix/internal/config"
	"github.com/get-vix/vix/internal/protocol"
)

// A session that starts (connects) but never sends a message must NOT be
// persisted to open/. Persisting a message-less session on connect leaves a
// ghost empty record when the connection drops before the first message.
func TestSessionStart_MessagelessNotPersisted(t *testing.T) {
	dir := t.TempDir()
	srv := newInstanceTestServer(t)
	srv.SetVersion("v1.2.3")
	_, cancel := serve(t, srv)
	defer cancel()

	conn, err := net.Dial("unix", srv.sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	data, _ := json.Marshal(protocol.SessionStartData{
		CWD:           dir,
		ConfigDir:     dir,
		ClientVersion: "v1.2.3",
	})
	payload, _ := json.Marshal(protocol.SessionCommand{Type: "session.start", Data: data})
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		t.Fatalf("write session.start: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var ev protocol.SessionEvent
	if err := json.NewDecoder(conn).Decode(&ev); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Type != "event.session_started" {
		t.Fatalf("got event %q, want event.session_started", ev.Type)
	}

	// No record should exist for this message-less session.
	paths := config.NewVixPaths(dir, srv.homeVixDir, dir)
	if recs := listOpenSessionRecords(paths); len(recs) != 0 {
		t.Fatalf("message-less session persisted a record (%d found); a ghost empty session was created", len(recs))
	}
}
