import AppKit
import SwiftUI

/// NSTextView-backed karaoke display — efficient highlight updates without rebuilding AttributedString.
struct KaraokeTextView: NSViewRepresentable {
    let text: String
    let wordRanges: [NSRange]
    let activeWordIndex: Int?

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    func makeNSView(context: Context) -> NSScrollView {
        let scroll = NSScrollView()
        scroll.hasVerticalScroller = true
        scroll.hasHorizontalScroller = false
        scroll.borderType = .noBorder
        scroll.drawsBackground = true
        scroll.backgroundColor = .textBackgroundColor

        let textView = KaraokeNSTextView()
        textView.isEditable = false
        textView.isSelectable = true
        textView.drawsBackground = true
        textView.backgroundColor = .textBackgroundColor
        textView.textContainerInset = NSSize(width: 8, height: 8)
        textView.font = .systemFont(ofSize: NSFont.systemFontSize)
        textView.textColor = .labelColor
        textView.isVerticallyResizable = true
        textView.isHorizontallyResizable = false
        textView.autoresizingMask = [.width]
        textView.textContainer?.widthTracksTextView = true
        textView.textContainer?.containerSize = NSSize(
            width: scroll.contentSize.width,
            height: .greatestFiniteMagnitude
        )

        scroll.documentView = textView
        context.coordinator.textView = textView
        return scroll
    }

    func updateNSView(_ scrollView: NSScrollView, context: Context) {
        guard let textView = context.coordinator.textView else { return }

        if textView.string != text {
            textView.string = text
            context.coordinator.lastActiveIndex = nil
        }

        let highlightIndex = resolvedHighlightIndex()
        guard highlightIndex != context.coordinator.lastActiveIndex else { return }
        context.coordinator.lastActiveIndex = highlightIndex
        context.coordinator.applyHighlight(
            in: text,
            wordRanges: wordRanges,
            activeIndex: highlightIndex
        )

        if let highlightIndex,
           highlightIndex < wordRanges.count,
           wordRanges[highlightIndex].location != NSNotFound {
            textView.scrollRangeToVisible(wordRanges[highlightIndex])
        }
    }

    /// Skip timestamp indices whose source range could not be mapped.
    private func resolvedHighlightIndex() -> Int? {
        guard let idx = activeWordIndex else { return nil }
        if idx < wordRanges.count, wordRanges[idx].location != NSNotFound {
            return idx
        }
        return wordRanges.indices.first { $0 >= idx && wordRanges[$0].location != NSNotFound }
    }

    final class Coordinator {
        weak var textView: KaraokeNSTextView?
        var lastActiveIndex: Int?

        private let baseAttributes: [NSAttributedString.Key: Any] = [
            .font: NSFont.systemFont(ofSize: NSFont.systemFontSize),
            .foregroundColor: NSColor.labelColor,
        ]

        private let highlightAttributes: [NSAttributedString.Key: Any] = [
            .font: NSFont.boldSystemFont(ofSize: NSFont.systemFontSize),
            .foregroundColor: NSColor.labelColor,
            .backgroundColor: NSColor.controlAccentColor.withAlphaComponent(0.45),
        ]

        func applyHighlight(in text: String, wordRanges: [NSRange], activeIndex: Int?) {
            guard let textView, let storage = textView.textStorage else { return }
            let full = NSRange(location: 0, length: (text as NSString).length)
            storage.setAttributes(baseAttributes, range: full)

            guard let activeIndex,
                  activeIndex < wordRanges.count,
                  wordRanges[activeIndex].location != NSNotFound
            else { return }

            let range = wordRanges[activeIndex]
            guard NSMaxRange(range) <= full.length else { return }
            storage.addAttributes(highlightAttributes, range: range)
        }
    }
}

/// Suppresses the focus ring when embedded in SwiftUI.
final class KaraokeNSTextView: NSTextView {
    override var focusRingType: NSFocusRingType {
        get { .none }
        set {}
    }
}
