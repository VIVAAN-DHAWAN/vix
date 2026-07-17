import Foundation
import Observation
import VixClient
import VixProtocol

/// Observable session controller for the SwiftUI app: owns the connection, the
/// event-consuming task, and the reduced transcript state. All UI-facing state
/// mutates on the main actor.
@MainActor
@Observable
public final class SessionModel {
    public enum Connection: Equatable, Sendable {
        case disconnected
        case connecting
        case connected(version: String)
        case failed(String)
    }

    public private(set) var state = TranscriptState()
    public private(set) var connection: Connection = .disconnected
    public var inputText = ""

    public let cwd: String
    private let client: VixSessionClient
    private var streamTask: Task<Void, Never>?

    public init(client: VixSessionClient = VixSessionClient(),
                cwd: String = FileManager.default.currentDirectoryPath) {
        self.client = client
        self.cwd = cwd
    }

    /// Ping the daemon (dev-friendly version discovery), open a session, and
    /// begin consuming events into `state`.
    public func connect() {
        guard connection == .disconnected || isFailed else { return }
        connection = .connecting
        do {
            let ping = try client.ping()
            guard ping.ok else {
                connection = .failed("vixd is not responding on \(client.socketPath)")
                return
            }
            let events = try client.start(cwd: cwd, clientVersion: ping.version)
            connection = .connected(version: ping.version)
            streamTask = Task { @MainActor [weak self] in
                do {
                    for try await event in events {
                        self?.apply(event)
                    }
                } catch {
                    self?.connection = .failed("\(error)")
                }
            }
        } catch {
            connection = .failed("cannot reach vixd at \(client.socketPath): \(error)")
        }
    }

    /// Send the current input as a user turn.
    public func send() {
        let text = inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, case .connected = connection else { return }
        inputText = ""
        state.items.append(.user(id: UUID(), text: text))
        do {
            try client.sendInput(text)
        } catch {
            state.items.append(.notice(id: UUID(), text: "send failed: \(error)"))
        }
    }

    public func cancel() {
        try? client.cancel()
    }

    public func disconnect() {
        streamTask?.cancel()
        streamTask = nil
        client.closeSession()
        connection = .disconnected
    }

    private func apply(_ event: SessionEvent) {
        reduce(&state, event)
    }

    private var isFailed: Bool {
        if case .failed = connection { return true }
        return false
    }
}
