import Foundation

/// Cleans pasted / Word-imported text for TTS and karaoke alignment.
enum SpeechText {
    /// Plain text safe for Kokoro and highlight mapping.
    static func sanitize(_ raw: String) -> String {
        var s = raw
            .replacingOccurrences(of: "\u{FFFC}", with: "") // Word inline object
            .replacingOccurrences(of: "\u{200B}", with: "")
            .replacingOccurrences(of: "\u{FEFF}", with: "")
            .replacingOccurrences(of: "\u{00A0}", with: " ")
            .replacingOccurrences(of: "\u{2028}", with: "\n")
            .replacingOccurrences(of: "\u{2029}", with: "\n")

        // Smart quotes & dashes → ASCII equivalents
        let replacements: [(String, String)] = [
            ("\u{2018}", "'"), ("\u{2019}", "'"), ("\u{201B}", "'"),
            ("\u{201C}", "\""), ("\u{201D}", "\""),
            ("\u{2013}", "-"), ("\u{2014}", "-"),
            ("\u{2026}", "..."),
        ]
        for (from, to) in replacements {
            s = s.replacingOccurrences(of: from, with: to)
        }

        // Word paragraph marks and legacy control chars → space or newline
        s = s.replacingOccurrences(of: "\r\n", with: "\n")
        s = s.replacingOccurrences(of: "\r", with: "\n")

        // Collapse runs of spaces (keep newlines)
        s = collapseWhitespace(s)

        return s.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Normalized form for fuzzy word matching.
    static func normalizeToken(_ word: String) -> String {
        word.lowercased()
            .unicodeScalars
            .filter { CharacterSet.alphanumerics.contains($0) }
            .map(String.init)
            .joined()
    }

    private static func collapseWhitespace(_ text: String) -> String {
        var out = ""
        var lastWasSpace = false
        for ch in text {
            if ch.isNewline {
                if !out.isEmpty, !out.hasSuffix("\n") {
                    out.append("\n")
                }
                lastWasSpace = false
            } else if ch.isWhitespace {
                if !lastWasSpace, !out.isEmpty, !out.hasSuffix("\n") {
                    out.append(" ")
                    lastWasSpace = true
                }
            } else {
                out.append(ch)
                lastWasSpace = false
            }
        }
        return out
    }
}

struct TextToken {
    let text: String
    let range: NSRange
}

struct TextAlignmentResult {
    let displayText: String
    let ranges: [NSRange]
}

/// Maps Kokoro timestamp words onto character ranges in the display text.
enum TextAlignment {
    static func align(text: String, timestamps: [WordTimestamp]) -> TextAlignmentResult {
        let sanitized = SpeechText.sanitize(text)
        guard !sanitized.isEmpty, !timestamps.isEmpty else {
            return TextAlignmentResult(displayText: sanitized, ranges: [])
        }

        let tokens = tokenize(sanitized)
        var ranges: [NSRange] = []
        var tokenIdx = 0

        for ts in timestamps {
            let target = SpeechText.normalizeToken(ts.word)

            if target.isEmpty {
                if let r = matchPunctuation(ts.word, in: sanitized, tokens: tokens, from: &tokenIdx, ranges: ranges) {
                    ranges.append(r)
                    continue
                }
                ranges.append(NSRange(location: NSNotFound, length: 0))
                continue
            }

            var matched = false
            while tokenIdx < tokens.count {
                let tok = tokens[tokenIdx]
                tokenIdx += 1
                let norm = SpeechText.normalizeToken(tok.text)
                if norm == target || norm.hasPrefix(target) || target.hasPrefix(norm) {
                    ranges.append(tok.range)
                    matched = true
                    break
                }
            }
            if !matched {
                ranges.append(NSRange(location: NSNotFound, length: 0))
            }
        }

        let hits = ranges.filter { $0.location != NSNotFound }.count
        if hits < max(1, timestamps.count / 2) {
            return buildFromTimestamps(timestamps)
        }
        return TextAlignmentResult(displayText: sanitized, ranges: ranges)
    }

    private static func matchPunctuation(
        _ punct: String,
        in text: String,
        tokens: [TextToken],
        from tokenIdx: inout Int,
        ranges: [NSRange]
    ) -> NSRange? {
        guard let ch = punct.unicodeScalars.first, punct.count == 1 else { return nil }
        let searchFrom: String.Index
        if let last = ranges.last(where: { $0.location != NSNotFound }),
           let end = Range(last, in: text)?.upperBound {
            searchFrom = end
        } else if tokenIdx < tokens.count {
            searchFrom = Range(tokens[tokenIdx].range, in: text)?.lowerBound ?? text.startIndex
        } else {
            searchFrom = text.startIndex
        }
        var i = searchFrom
        while i < text.endIndex {
            if text[i].unicodeScalars.first == ch {
                let start = i
                let end = text.index(after: i)
                let r = NSRange(start..<end, in: text)
                // advance tokenIdx past any token containing this position
                while tokenIdx < tokens.count,
                      tokens[tokenIdx].range.location + tokens[tokenIdx].range.length <= r.location + r.length {
                    tokenIdx += 1
                }
                return r
            }
            if !text[i].isWhitespace { break }
            i = text.index(after: i)
        }
        return nil
    }

    private static func tokenize(_ text: String) -> [TextToken] {
        var tokens: [TextToken] = []
        var i = text.startIndex
        while i < text.endIndex {
            while i < text.endIndex, text[i].isWhitespace, !text[i].isNewline { i = text.index(after: i) }
            if i >= text.endIndex { break }

            let start = i
            while i < text.endIndex, !text[i].isWhitespace {
                i = text.index(after: i)
            }
            let range = NSRange(start..<i, in: text)
            let slice = String(text[start..<i])
            tokens.append(TextToken(text: slice, range: range))
        }
        return tokens
    }

    /// Fallback: display text built from Kokoro's own timestamp words (always in sync).
    private static func buildFromTimestamps(_ timestamps: [WordTimestamp]) -> TextAlignmentResult {
        var display = ""
        var ranges: [NSRange] = []
        for (idx, ts) in timestamps.enumerated() {
            let word = ts.word
            guard !word.isEmpty else {
                ranges.append(NSRange(location: NSNotFound, length: 0))
                continue
            }
            if !display.isEmpty, !word.allSatisfy({ $0.isWhitespace || $0.isNewline }) {
                let last = display.unicodeScalars.last
                if last != nil, !CharacterSet.whitespacesAndNewlines.contains(last!) {
                    display.append(" ")
                }
            }
            let start = display.utf16.count
            display.append(word)
            let len = word.utf16.count
            if len > 0 {
                ranges.append(NSRange(location: start, length: len))
            } else {
                ranges.append(NSRange(location: NSNotFound, length: 0))
            }
            _ = idx
        }
        return TextAlignmentResult(displayText: display, ranges: ranges)
    }
}
