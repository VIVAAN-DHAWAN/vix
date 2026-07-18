import SwiftUI
import VixMacCore

struct ContentView: View {
    @Bindable var model: SessionModel

    var body: some View {
        HStack(spacing: 0) {
            VStack(spacing: 0) {
                header
                Divider()
                if let banner = model.banner {
                    HStack(spacing: 6) {
                        Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                        Text(banner).font(.callout)
                        Spacer()
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 6)
                    .background(.orange.opacity(0.12))
                    Divider()
                }
                transcript
                Divider()
                inputBar
            }
            if !model.state.todos.isEmpty {
                Divider()
                TodoPanelView(todos: model.state.todos)
                    .frame(width: 240)
            }
        }
        .sheet(isPresented: pendingBinding) {
            InteractionSheet(model: model).interactiveDismissDisabled()
        }
    }

    // Presented whenever a blocking round-trip is awaiting the user. The setter
    // is a no-op: the sheet dismisses by answering, which clears model.pending.
    private var pendingBinding: Binding<Bool> {
        Binding(get: { model.state.pending != nil }, set: { _ in })
    }

    // MARK: Header

    private var header: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(statusColor)
                .frame(width: 8, height: 8)
            Text(model.state.title.isEmpty ? "vix" : model.state.title)
                .font(.headline)
            Spacer()
            if case .failed = model.connection {
                Button("Reconnect") { model.retry() }
                    .buttonStyle(.borderless)
                    .font(.caption)
            }
            Text(statusText)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
    }

    private var statusColor: Color {
        switch model.connection {
        case .connected: return .green
        case .connecting: return .yellow
        case .failed: return .red
        case .disconnected: return .gray
        }
    }

    private var statusText: String {
        switch model.connection {
        case .disconnected: return "disconnected"
        case .connecting: return "connecting…"
        case .connected(let v): return "vixd \(v)"
        case .failed(let m): return m
        }
    }

    // MARK: Transcript

    private var transcript: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 10) {
                    ForEach(model.state.items) { item in
                        row(for: item).id(item.id)
                    }
                    if model.state.isStreaming {
                        ProgressView().controlSize(.small).id("streaming")
                    }
                }
                .padding(12)
            }
            .onChange(of: model.state.items.count) {
                if let last = model.state.items.last {
                    withAnimation { proxy.scrollTo(last.id, anchor: .bottom) }
                }
            }
        }
    }

    @ViewBuilder
    private func row(for item: TranscriptItem) -> some View {
        switch item {
        case .user(_, let text):
            messageBubble(text, role: "You", color: .accentColor.opacity(0.15), align: .trailing)
        case .assistant(_, let text):
            messageBubble(markdown(text), role: "vix", color: .gray.opacity(0.12), align: .leading)
        case .thinking(_, let text):
            Text(text)
                .font(.callout)
                .italic()
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
        case .tool(_, let tool):
            toolRow(tool)
        case .notice(_, let text):
            Text(text)
                .font(.callout)
                .foregroundStyle(.red)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func messageBubble(_ content: some View, role: String, color: Color, align: HorizontalAlignment) -> some View {
        VStack(alignment: align, spacing: 2) {
            Text(role).font(.caption2).foregroundStyle(.secondary)
            content
                .padding(8)
                .background(color, in: RoundedRectangle(cornerRadius: 8))
                .textSelection(.enabled)
        }
        .frame(maxWidth: .infinity, alignment: align == .trailing ? .trailing : .leading)
    }

    private func messageBubble(_ text: String, role: String, color: Color, align: HorizontalAlignment) -> some View {
        messageBubble(Text(text), role: role, color: color, align: align)
    }

    private func toolRow(_ tool: ToolRow) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                Image(systemName: tool.done ? (tool.isError ? "xmark.circle" : "checkmark.circle") : "gearshape")
                    .foregroundStyle(tool.isError ? .red : .secondary)
                Text(tool.name).font(.callout.monospaced()).bold()
                Text(tool.summary).font(.callout).foregroundStyle(.secondary)
            }
            if tool.done, !tool.output.isEmpty {
                Text(tool.output.prefix(2000))
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(8)
                    .textSelection(.enabled)
            }
        }
        .padding(8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 8))
    }

    private func markdown(_ text: String) -> Text {
        if let attr = try? AttributedString(markdown: text, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)) {
            return Text(attr)
        }
        return Text(text)
    }

    // MARK: Input

    private var inputBar: some View {
        HStack(spacing: 8) {
            TextField("Message vix…", text: $model.inputText, axis: .vertical)
                .textFieldStyle(.plain)
                .lineLimit(1...6)
                .onSubmit { model.send() }
                .disabled(!isConnected)
            if model.state.isStreaming {
                Button(role: .cancel) { model.cancel() } label: {
                    Image(systemName: "stop.circle.fill")
                }
                .buttonStyle(.borderless)
            } else {
                Button { model.send() } label: {
                    Image(systemName: "arrow.up.circle.fill")
                }
                .buttonStyle(.borderless)
                .disabled(!isConnected || model.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(10)
    }

    private var isConnected: Bool {
        if case .connected = model.connection { return true }
        return false
    }
}
