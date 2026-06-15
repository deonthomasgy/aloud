import AppKit
import SwiftUI

/// Menu bar icon — template image (transparent, adapts to light/dark menu bar).
enum AppIcons {
    static var menuBar: NSImage? {
        if let url = Bundle.main.url(forResource: "MenuBarIcon", withExtension: "png"),
           let img = NSImage(contentsOf: url) {
            img.isTemplate = true
            img.size = NSSize(width: 18, height: 18)
            return img
        }
        return nil
    }
}

/// Brings the main invtts window to the front.
enum AppWindow {
    static func show() {
        NSApp.activate(ignoringOtherApps: true)
        for window in NSApp.windows where window.canBecomeMain {
            window.makeKeyAndOrderFront(nil)
            return
        }
    }
}
