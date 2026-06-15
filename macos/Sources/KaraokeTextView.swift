import SwiftUI

/// SwiftUI text view with live word highlight during playback.
/// Equatable so playback-time slider ticks don't rebuild the full attributed string.
struct KaraokeTextView: View, Equatable {
    let text: String
    let wordRanges: [NSRange]
    let activeWordIndex: Int?

    var body: some View {
        ScrollView {
            Text(highlightedText)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(8)
                .textSelection(.enabled)
        }
        .frame(minHeight: 160)
    }

    static func == (lhs: KaraokeTextView, rhs: KaraokeTextView) -> Bool {
        lhs.text == rhs.text
            && lhs.activeWordIndex == rhs.activeWordIndex
            && lhs.wordRanges.count == rhs.wordRanges.count
    }

    private var highlightedText: AttributedString {
        var attributed = AttributedString(text)
        guard let idx = activeWordIndex,
              idx < wordRanges.count,
              wordRanges[idx].location != NSNotFound,
              let swiftRange = Range(wordRanges[idx], in: text),
              let start = AttributedString.Index(swiftRange.lowerBound, within: attributed),
              let end = AttributedString.Index(swiftRange.upperBound, within: attributed)
        else {
            return attributed
        }
        attributed[start..<end].backgroundColor = Color.accentColor.opacity(0.5)
        attributed[start..<end].font = .body.bold()
        return attributed
    }
}
