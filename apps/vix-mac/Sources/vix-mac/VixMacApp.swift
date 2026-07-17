import SwiftUI
import VixMacCore

@main
struct VixMacApp: App {
    @State private var model = SessionModel()

    init() {
        // Help `swift run` (no bundle) show a normal, focusable window.
        NSApplication.shared.setActivationPolicy(.regular)
    }

    var body: some Scene {
        WindowGroup {
            ContentView(model: model)
                .frame(minWidth: 640, minHeight: 480)
                .task { model.connect() }
        }
        .windowStyle(.titleBar)
    }
}
