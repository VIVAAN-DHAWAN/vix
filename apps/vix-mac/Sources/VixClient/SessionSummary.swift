import Foundation

/// Lightweight projection of a persisted session returned by the `session.list`
/// RPC. This is an RPC response type (not part of the event/command envelope
/// surface), so unlike the generated models it is hand-written here. Keep the
/// JSON keys in sync with protocol.SessionSummary.
public struct SessionSummary: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let cwd: String
    public let model: String
    public let title: String?
    public let firstMessage: String?
    public let startedAt: String?
    public let lastRequestAt: String?
    public let attached: Bool?
    public let origin: String?
    public let jobStatus: String?
    public let unread: Bool?

    enum CodingKeys: String, CodingKey {
        case id, cwd, model, title
        case firstMessage = "first_message"
        case startedAt = "started_at"
        case lastRequestAt = "last_request_at"
        case attached, origin
        case jobStatus = "job_status"
        case unread
    }

    /// Display label: title, else first message, else the id.
    public var displayTitle: String {
        if let t = title, !t.isEmpty { return t }
        if let f = firstMessage, !f.isEmpty { return f }
        return id
    }

    /// True for vix-initiated sessions (scheduled jobs, alerts).
    public var isVixInitiated: Bool { origin == "vix" }
}
