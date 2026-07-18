import Foundation
import VixProtocol

// SessionSummary is now generated into VixProtocol from the protocol schema
// (make mac-models). This extension adds the client-side conveniences the app
// needs on top of the generated fields.

extension SessionSummary: Identifiable {}

extension SessionSummary {
    /// Display label: title, else first message, else the id.
    public var displayTitle: String {
        if let t = title, !t.isEmpty { return t }
        if let f = firstMessage, !f.isEmpty { return f }
        return id
    }

    /// True for vix-initiated sessions (scheduled jobs, alerts).
    public var isVixInitiated: Bool { origin == "vix" }
}
