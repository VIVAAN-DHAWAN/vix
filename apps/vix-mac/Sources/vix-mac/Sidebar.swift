import SwiftUI
import VixMacCore
import VixClient
import VixProtocol

/// The session sidebar, grouped into user sessions and vix-initiated runs.
struct SessionListView: View {
    @Bindable var app: AppModel

    var body: some View {
        List(selection: selection) {
            let userSessions = app.sessions.filter { !$0.isVixInitiated }
            let vixSessions = app.sessions.filter { $0.isVixInitiated }

            if !userSessions.isEmpty {
                Section("Sessions") {
                    ForEach(userSessions) { row($0) }
                }
            }
            if !vixSessions.isEmpty {
                Section("Vix-initiated") {
                    ForEach(vixSessions) { row($0) }
                }
            }
        }
        .navigationTitle("vix")
        .toolbar {
            ToolbarItem {
                Button { app.newSession() } label: { Image(systemName: "square.and.pencil") }
                    .help("New session")
            }
            ToolbarItem {
                Button { app.refresh() } label: { Image(systemName: "arrow.clockwise") }
                    .help("Refresh sessions")
            }
        }
    }

    private var selection: Binding<String?> {
        Binding(
            get: { app.selectedID },
            set: { id in
                // Defer out of the view-update cycle: opening mutates AppModel
                // state and must not run synchronously inside the List's binding.
                if let id, let summary = app.sessions.first(where: { $0.id == id }) {
                    Task { @MainActor in app.open(summary) }
                }
            })
    }

    private func row(_ summary: SessionSummary) -> some View {
        HStack(spacing: 6) {
            VStack(alignment: .leading, spacing: 2) {
                Text(summary.displayTitle).lineLimit(1)
                if !summary.model.isEmpty {
                    Text(summary.model).font(.caption2).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            Spacer()
            if summary.unread == true {
                Circle().fill(.blue).frame(width: 7, height: 7)
            }
        }
        .tag(summary.id)
    }
}

/// The todo panel (right column of the chat pane).
struct TodoPanelView: View {
    let todos: [TodoItem]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Todos").font(.headline)
            ForEach(todos, id: \.id) { todo in
                HStack(alignment: .top, spacing: 6) {
                    Image(systemName: icon(todo.status))
                        .foregroundStyle(color(todo.status))
                    Text(todo.content)
                        .font(.callout)
                        .strikethrough(todo.status == "completed")
                        .foregroundStyle(todo.status == "completed" ? .secondary : .primary)
                }
            }
            Spacer()
        }
        .padding(12)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(.quaternary.opacity(0.15))
    }

    private func icon(_ status: String) -> String {
        switch status {
        case "completed": return "checkmark.circle.fill"
        case "in_progress": return "circle.dotted"
        default: return "circle"
        }
    }

    private func color(_ status: String) -> Color {
        switch status {
        case "completed": return .green
        case "in_progress": return .blue
        default: return .secondary
        }
    }
}
