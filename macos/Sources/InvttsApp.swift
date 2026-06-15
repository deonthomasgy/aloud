import SwiftUI

@main
struct InvttsApp: App {
    @StateObject private var model = TTSViewModel()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(model)
                .frame(minWidth: 520, minHeight: 640)
        }
        .windowStyle(.hiddenTitleBar)
        .windowToolbarStyle(.unified)
        .commands {
            CommandGroup(replacing: .newItem) {}
            CommandMenu("Speech") {
                Button("Generate") {
                    model.generate()
                }
                .keyboardShortcut(.return, modifiers: .command)
                .disabled(model.isGenerating || model.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                Button("Cancel") {
                    model.cancelGenerate()
                }
                .disabled(!model.isGenerating)
            }
            CommandMenu("Import") {
                Button("From Word — Selection") {
                    model.importFromWord(mode: .selection)
                }
                .keyboardShortcut("i", modifiers: [.command, .shift])
                Button("From Word — Entire Document") {
                    model.importFromWord(mode: .activeDocument)
                }
                .keyboardShortcut("i", modifiers: [.command, .option])
            }
        }

        MenuBarExtra {
            menuBarContent
        } label: {
            menuBarLabel
        }

        Settings {
            SettingsView()
                .environmentObject(model)
        }
    }

    @ViewBuilder
    private var menuBarLabel: some View {
        Image(systemName: "waveform")
    }

    @ViewBuilder
    private var menuBarContent: some View {
        Button("Open invtts") {
            AppWindow.show()
        }
        Divider()
        Button("Import Word Selection") {
            model.importFromWord(mode: .selection)
            AppWindow.show()
        }
        .disabled(!WordImporter.isInstalled)
        Button("Import Word Document") {
            model.importFromWord(mode: .activeDocument)
            AppWindow.show()
        }
        .disabled(!WordImporter.isInstalled)
        Divider()
        Button("Generate Speech") {
            AppWindow.show()
            model.generate()
        }
        .disabled(model.isGenerating || model.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        Button("Cancel Synthesis") {
            model.cancelGenerate()
        }
        .disabled(!model.isGenerating)
        Divider()
        Button("Quit invtts") {
            NSApp.terminate(nil)
        }
    }
}
