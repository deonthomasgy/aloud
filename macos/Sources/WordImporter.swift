import AppKit
import Foundation

/// Imports text from Microsoft Word via AppleScript (macOS).
enum WordImporter {
    enum ImportMode {
        case selection
        case activeDocument
    }

    enum ImportError: LocalizedError {
        case wordNotInstalled
        case noDocument
        case emptyContent
        case scriptFailed(String)

        var errorDescription: String? {
            switch self {
            case .wordNotInstalled:
                return "Microsoft Word is not installed."
            case .noDocument:
                return "No open Word document. Open a document and try again."
            case .emptyContent:
                return "Word returned no text. Select some text or open a document with content."
            case .scriptFailed(let detail):
                return detail
            }
        }
    }

    static var isInstalled: Bool {
        NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.microsoft.Word") != nil
    }

    static func importText(mode: ImportMode) throws -> String {
        guard isInstalled else { throw ImportError.wordNotInstalled }
        let raw = try runImportScript(mode: mode)
        let cleaned = SpeechText.sanitize(raw)
        if cleaned.isEmpty { throw ImportError.emptyContent }
        return cleaned
    }

    private static func runImportScript(mode: ImportMode) throws -> String {
        let script: String
        switch mode {
        case .selection:
            script = """
            tell application "Microsoft Word"
                if not (exists active document) then error "No document"
                set docText to content of text object of selection
                if docText is missing value then return ""
                return docText as text
            end tell
            """
        case .activeDocument:
            script = """
            tell application "Microsoft Word"
                if not (exists active document) then error "No document"
                set docText to content of text object of active document
                if docText is missing value then return ""
                return docText as text
            end tell
            """
        }
        return try runAppleScript(script)
    }

    private static func runAppleScript(_ source: String) throws -> String {
        var error: NSDictionary?
        guard let script = NSAppleScript(source: source) else {
            throw ImportError.scriptFailed("Could not compile AppleScript.")
        }
        let output = script.executeAndReturnError(&error)
        if let error {
            let msg = (error[NSAppleScript.errorMessage] as? String) ?? "AppleScript failed."
            if msg.localizedCaseInsensitiveContains("No document") {
                throw ImportError.noDocument
            }
            if msg.localizedCaseInsensitiveContains("not running")
                || msg.localizedCaseInsensitiveContains("Application isn't running")
                || msg.localizedCaseInsensitiveContains("Application isn’t running") {
                try launchWord()
                return try runAppleScript(source)
            }
            throw ImportError.scriptFailed(msg)
        }
        return output.stringValue ?? ""
    }

    private static func launchWord() throws {
        guard let url = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.microsoft.Word") else {
            throw ImportError.wordNotInstalled
        }
        let config = NSWorkspace.OpenConfiguration()
        config.activates = true
        let sem = DispatchSemaphore(value: 0)
        var launchError: Error?
        NSWorkspace.shared.openApplication(at: url, configuration: config) { _, err in
            launchError = err
            sem.signal()
        }
        sem.wait()
        if let launchError { throw ImportError.scriptFailed(launchError.localizedDescription) }
        Thread.sleep(forTimeInterval: 1.0)
    }
}
