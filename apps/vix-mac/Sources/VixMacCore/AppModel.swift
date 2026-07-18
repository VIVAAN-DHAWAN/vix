import Foundation
import Observation
import VixClient
import VixProtocol

/// Top-level coordinator: owns the persisted session list and the currently
/// active session. Views observe this to drive the sidebar and detail panes.
@MainActor
@Observable
public final class AppModel {
    public private(set) var sessions: [SessionSummary] = []
    public private(set) var active: SessionModel?
    public private(set) var selectedID: String?

    public let cwd: String
    private let socketPath: String
    private let authToken: String?

    public init(socketPath: String = VixSessionClient.defaultSocketPath,
                authToken: String? = nil,
                cwd: String = FileManager.default.currentDirectoryPath) {
        self.socketPath = socketPath
        self.authToken = authToken
        self.cwd = cwd
    }

    private func makeClient() -> VixSessionClient {
        VixSessionClient(socketPath: socketPath, authToken: authToken)
    }

    /// Refresh the persisted session list (best-effort; off the main thread).
    public func refresh() {
        let client = makeClient()
        let cwd = self.cwd
        Task { [weak self] in
            let list = await Task.detached { (try? client.listSessions(cwd: cwd)) ?? [] }.value
            self?.sessions = list
        }
    }

    /// Start a fresh session and make it active.
    public func newSession() {
        active?.disconnect()
        let model = SessionModel(client: makeClient(), cwd: cwd)
        active = model
        selectedID = nil
        model.connect()
    }

    /// Attach to an existing persisted session and make it active.
    public func open(_ summary: SessionSummary) {
        guard summary.id != selectedID else { return }
        active?.disconnect()
        let model = SessionModel(client: makeClient(), cwd: cwd)
        active = model
        selectedID = summary.id
        model.attach(sessionID: summary.id)
    }
}
