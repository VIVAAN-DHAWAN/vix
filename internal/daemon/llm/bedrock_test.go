package llm

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
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

// --- request replay: thinking blocks -------------------------------------

// decodeRequest builds a request and returns its parsed messages array.
func decodeRequest(t *testing.T, messages []MessageParam) []map[string]any {
	t.Helper()
	raw, err := testBedrockClient().buildRequest(nil, messages, nil, 1024)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	var got struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return got.Messages
}

func blockTypes(t *testing.T, msg map[string]any) []string {
	t.Helper()
	var out []string
	content, _ := msg["content"].([]any)
	for _, c := range content {
		cm, _ := c.(map[string]any)
		out = append(out, fmt.Sprint(cm["type"]))
	}
	return out
}

// Bedrock returns thinking blocks carrying a signature but no text. Replaying
// one emits {"type":"thinking","signature":...} with no `thinking` key, because
// bdContent tags it omitempty — and the API rejects that with
// "messages.N.content.0.thinking.thinking: Field required", killing every
// request after the first in a thread.
func TestBuildRequestDropsSignatureOnlyThinkingBlock(t *testing.T) {
	msgs := []MessageParam{
		{Role: RoleUser, Content: []ContentBlock{{Type: BlockText, Text: "hi"}}},
		{Role: RoleAssistant, Content: []ContentBlock{
			{Type: BlockThinking, Text: "", Signature: "c2lnbmF0dXJl"},
			{Type: BlockText, Text: "hello"},
		}},
		{Role: RoleUser, Content: []ContentBlock{{Type: BlockText, Text: "again"}}},
	}
	got := decodeRequest(t, msgs)
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	if types := blockTypes(t, got[1]); len(types) != 1 || types[0] != "text" {
		t.Errorf("assistant blocks = %v, want only [text]", types)
	}
}

// A thinking block that actually has text is still replayed, with the
// `thinking` field present — the fix must not strip usable reasoning.
func TestBuildRequestKeepsThinkingWithText(t *testing.T) {
	msgs := []MessageParam{
		{Role: RoleUser, Content: []ContentBlock{{Type: BlockText, Text: "hi"}}},
		{Role: RoleAssistant, Content: []ContentBlock{
			{Type: BlockThinking, Text: "let me think", Signature: "c2ln"},
			{Type: BlockText, Text: "hello"},
		}},
	}
	got := decodeRequest(t, msgs)
	types := blockTypes(t, got[1])
	if len(types) != 2 || types[0] != "thinking" {
		t.Fatalf("assistant blocks = %v, want [thinking text]", types)
	}
	content, _ := got[1]["content"].([]any)
	first, _ := content[0].(map[string]any)
	if first["thinking"] != "let me think" {
		t.Errorf("thinking field = %v, want %q", first["thinking"], "let me think")
	}
}

// Stripping the only block would leave an empty content array, which the API
// rejects in turn — the message must be dropped instead.
func TestBuildRequestSkipsMessageEmptiedByStripping(t *testing.T) {
	msgs := []MessageParam{
		{Role: RoleUser, Content: []ContentBlock{{Type: BlockText, Text: "hi"}}},
		{Role: RoleAssistant, Content: []ContentBlock{
			{Type: BlockThinking, Text: "", Signature: "c2ln"},
		}},
	}
	got := decodeRequest(t, msgs)
	if len(got) != 1 {
		t.Fatalf("expected the emptied assistant message to be dropped, got %d messages", len(got))
	}
	for _, m := range got {
		if content, _ := m["content"].([]any); len(content) == 0 {
			t.Error("emitted a message with an empty content array")
		}
	}
}

// The cache breakpoint lands on the last block of the final user message. It
// must index the filtered slice, not the original.
func TestBuildRequestCacheControlIndexesFilteredBlocks(t *testing.T) {
	msgs := []MessageParam{
		{Role: RoleUser, Content: []ContentBlock{
			{Type: BlockThinking, Text: "", Signature: "c2ln"},
			{Type: BlockText, Text: "last"},
		}},
	}
	got := decodeRequest(t, msgs)
	content, _ := got[0]["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 block after stripping, got %d", len(content))
	}
	last, _ := content[0].(map[string]any)
	if _, ok := last["cache_control"]; !ok {
		t.Error("cache_control did not land on the surviving last block")
	}
}
