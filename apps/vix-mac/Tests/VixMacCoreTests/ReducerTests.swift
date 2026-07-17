import Foundation
import Testing
@testable import VixMacCore
import VixProtocol

private func event(_ type: String, _ data: JSONValue) -> SessionEvent {
    SessionEvent(data: data, type: type)
}

@Test func streamChunksAccumulateIntoOneAssistantMessage() {
    var s = TranscriptState()
    reduce(&s, event("event.stream_chunk", .object(["text": .string("Hello, ")])))
    reduce(&s, event("event.stream_chunk", .object(["text": .string("world")])))
    #expect(s.assistantText == "Hello, world")
    #expect(s.items.count == 1)
    #expect(s.isStreaming == true)
}

@Test func streamDoneRecordsTokensAndEndsStreaming() {
    var s = TranscriptState()
    reduce(&s, event("event.stream_chunk", .object(["text": .string("hi")])))
    reduce(&s, event("event.stream_done", .object([
        "input_tokens": .int(10), "output_tokens": .int(3),
        "cache_creation_tokens": .int(0), "cache_read_tokens": .int(0),
        "elapsed_ms": .int(42),
    ])))
    #expect(s.isStreaming == false)
    #expect(s.lastTokens?.input == 10)
    #expect(s.lastTokens?.output == 3)
}

@Test func toolResultMatchesCallByToolID() {
    var s = TranscriptState()
    reduce(&s, event("event.tool_call", .object([
        "tool_id": .string("t1"), "name": .string("bash"), "summary": .string("ls"),
    ])))
    reduce(&s, event("event.tool_result", .object([
        "tool_id": .string("t1"), "name": .string("bash"),
        "output": .string("file.txt"), "is_error": .bool(false),
    ])))
    #expect(s.toolRows.count == 1)
    let row = s.toolRows[0]
    #expect(row.toolID == "t1")
    #expect(row.done == true)
    #expect(row.output == "file.txt")
    #expect(row.isError == false)
}

@Test func toolErrorFlagsRow() {
    var s = TranscriptState()
    reduce(&s, event("event.tool_call", .object([
        "tool_id": .string("t9"), "name": .string("bash"), "summary": .string("boom"),
    ])))
    reduce(&s, event("event.tool_result", .object([
        "tool_id": .string("t9"), "name": .string("bash"),
        "output": .string("exit 1"), "is_error": .bool(true),
    ])))
    #expect(s.toolRows[0].isError == true)
}

@Test func interleavedToolCallSplitsAssistantText() {
    var s = TranscriptState()
    reduce(&s, event("event.stream_chunk", .object(["text": .string("before")])))
    reduce(&s, event("event.tool_call", .object([
        "tool_id": .string("t1"), "name": .string("bash"), "summary": .string("x"),
    ])))
    reduce(&s, event("event.stream_chunk", .object(["text": .string("after")])))
    // Two separate assistant rows around the tool row.
    let assistantCount = s.items.filter { if case .assistant = $0 { return true } else { return false } }.count
    #expect(assistantCount == 2)
    #expect(s.assistantText == "beforeafter")
}

@Test func errorEventAppendsNoticeAndEndsStreaming() {
    var s = TranscriptState()
    reduce(&s, event("event.stream_chunk", .object(["text": .string("partial")])))
    reduce(&s, event("event.error", .object(["message": .string("boom")])))
    #expect(s.isStreaming == false)
    if case .notice(_, let text) = s.items.last {
        #expect(text.contains("boom"))
    } else {
        Issue.record("expected a notice item")
    }
}

@Test func sessionStartedAndTitleUpdate() {
    var s = TranscriptState()
    reduce(&s, event("event.session_started", .object([
        "session_id": .string("sess-1"), "started_at": .string("2026-01-01T00:00:00Z"),
    ])))
    reduce(&s, event("event.title_updated", .object(["title": .string("My session")])))
    #expect(s.sessionID == "sess-1")
    #expect(s.title == "My session")
}

@Test func clearResetsTranscriptButKeepsSession() {
    var s = TranscriptState()
    reduce(&s, event("event.session_started", .object([
        "session_id": .string("sess-1"), "started_at": .string("x"),
    ])))
    reduce(&s, event("event.stream_chunk", .object(["text": .string("stuff")])))
    reduce(&s, event("event.clear", .null))
    #expect(s.items.isEmpty)
    #expect(s.sessionID == "sess-1")
}

@Test func agentDoneEndsStreaming() {
    var s = TranscriptState()
    reduce(&s, event("event.stream_chunk", .object(["text": .string("hi")])))
    reduce(&s, event("event.agent_done", .null))
    #expect(s.isStreaming == false)
}
