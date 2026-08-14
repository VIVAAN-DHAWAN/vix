package llm

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"
)

// encodeEventStreamFrame builds one Amazon EventStream frame carrying a single
// `:event-type` header. The CRC fields are left zero: readFrame does not verify
// them, and hand-computing them would only couple the fixture to a detail the
// parser ignores.
func encodeEventStreamFrame(eventType string, payload []byte) []byte {
	var hdr bytes.Buffer
	const name = ":event-type"
	hdr.WriteByte(byte(len(name)))
	hdr.WriteString(name)
	hdr.WriteByte(7) // value type 7 = string
	_ = binary.Write(&hdr, binary.BigEndian, uint16(len(eventType)))
	hdr.WriteString(eventType)
	headers := hdr.Bytes()

	total := 12 + len(headers) + len(payload) + 4
	var prelude [12]byte
	binary.BigEndian.PutUint32(prelude[0:4], uint32(total))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(headers)))

	out := make([]byte, 0, total)
	out = append(out, prelude[:]...)
	out = append(out, headers...)
	out = append(out, payload...)
	out = append(out, 0, 0, 0, 0) // message CRC
	return out
}

// chunkFrame wraps an Anthropic streaming event the way Bedrock does:
// {"bytes":"<base64 of the event JSON>"}.
func chunkFrame(t *testing.T, event map[string]any) []byte {
	t.Helper()
	inner, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal inner event: %v", err)
	}
	wrapper, err := json.Marshal(map[string]any{"bytes": inner})
	if err != nil {
		t.Fatalf("marshal wrapper: %v", err)
	}
	return encodeEventStreamFrame("chunk", wrapper)
}

// completeStream is exactly what Bedrock sends: content frames terminated by
// message_stop, then the body closes. There is deliberately no trailing
// empty-`:event-type` frame — Bedrock does not emit one.
func completeStream(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(chunkFrame(t, map[string]any{
		"type":    "message_start",
		"message": map[string]any{"usage": map[string]any{"input_tokens": 9}},
	}))
	buf.Write(chunkFrame(t, map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]any{"type": "text", "text": ""},
	}))
	buf.Write(chunkFrame(t, map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "text_delta", "text": text},
	}))
	buf.Write(chunkFrame(t, map[string]any{"type": "content_block_stop", "index": 0}))
	buf.Write(chunkFrame(t, map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": 3},
	}))
	buf.Write(chunkFrame(t, map[string]any{"type": "message_stop"}))
	return buf.Bytes()
}

func testBedrockClient() *bedrockClient {
	return &bedrockClient{streamIdleTimeout: 5 * time.Second}
}

// Bedrock closes the response body immediately after message_stop. Treating
// that EOF as a truncated stream discarded every completed response and drove
// the caller into its full retry budget, surfacing to the user as
// "Connection lost" with the correct answer already rendered.
func TestRunStreamAcceptsEOFAfterMessageStop(t *testing.T) {
	body := bytes.NewReader(completeStream(t, "Hello World"))

	msg, err := testBedrockClient().runStream(
		context.Background(), body, time.Now(), "test-req", nil, nil,
	)
	if err != nil {
		t.Fatalf("expected a completed message, got error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected a message, got nil")
	}
	if msg.TextContent != "Hello World" {
		t.Errorf("TextContent = %q, want %q", msg.TextContent, "Hello World")
	}
}

// The fix must stay narrow: an EOF with no message_stop is a genuine truncation
// and must remain a retryable error, otherwise a half-received answer would be
// silently returned as if it were complete.
func TestRunStreamStillFailsOnEOFWithoutMessageStop(t *testing.T) {
	full := completeStream(t, "Hello World")
	// Drop the trailing message_stop frame, leaving the stream genuinely cut short.
	stopFrame := chunkFrame(t, map[string]any{"type": "message_stop"})
	truncated := full[:len(full)-len(stopFrame)]

	_, err := testBedrockClient().runStream(
		context.Background(), bytes.NewReader(truncated), time.Now(), "test-req", nil, nil,
	)
	if err == nil {
		t.Fatal("expected a truncation error when message_stop never arrived, got nil")
	}
}
