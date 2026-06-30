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

    /// Filesystem-safe slug from the start of prepared speech text.
    static func filenameSlug(from text: String, maxWords: Int = 8, maxLength: Int = 56) -> String {
        let cleaned = sanitize(text)
        guard !cleaned.isEmpty else { return "" }

        let firstLine = cleaned
            .split(separator: "\n", maxSplits: 1, omittingEmptySubsequences: false)
            .first
            .map(String.init) ?? cleaned

        var slug = firstLine
            .split(whereSeparator: \.isWhitespace)
            .prefix(maxWords)
            .map { word -> String in
                word.lowercased()
                    .unicodeScalars
                    .filter { CharacterSet.alphanumerics.contains($0) }
                    .map(String.init)
                    .joined()
            }
            .filter { !$0.isEmpty }
            .joined(separator: "-")

        if slug.count > maxLength {
            slug = String(slug.prefix(maxLength))
                .trimmingCharacters(in: CharacterSet(charactersIn: "-"))
        }
        return slug
    }

    /// Default filename for a synthesized clip, e.g. `hello-world-this-is-a-test.mp3`.
    static func suggestedAudioFilename(text: String, voice: String, format: String) -> String {
        let slug = filenameSlug(from: text)
        let base = slug.isEmpty ? "speech-\(voice)" : slug
        return "\(base).\(format)"
    }

    /// Filename from an already-derived topic slug.
    static func suggestedAudioFilename(slug: String, format: String) -> String {
        let base = slug.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !base.isEmpty else { return "speech-clip.\(format)" }
        return "\(base).\(format)"
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

        let sequential = alignSequential(text: sanitized, timestamps: timestamps)
        let hits = sequential.ranges.filter { $0.location != NSNotFound }.count
        let threshold = max(1, (timestamps.count * 2) / 3)

        if hits >= threshold {
            return sequential
        }
        return buildFromTimestamps(timestamps)
    }

    /// Walk source text left-to-right, matching each Kokoro timestamp in order.
    /// Handles punctuation split across tokens (e.g. Kokoro: "world" + "," vs source "world,").
    private static func alignSequential(text: String, timestamps: [WordTimestamp]) -> TextAlignmentResult {
        let ns = text as NSString
        var ranges: [NSRange] = []
        var cursor = 0

        for ts in timestamps {
            let target = ts.word
            let normTarget = SpeechText.normalizeToken(target)

            if normTarget.isEmpty {
                if let range = findLiteral(target, in: ns, from: cursor) {
                    ranges.append(range)
                    cursor = range.location + range.length
                } else {
                    ranges.append(NSRange(location: NSNotFound, length: 0))
                }
                continue
            }

            if let (range, next) = findWord(normTarget, in: ns, from: cursor) {
                ranges.append(range)
                cursor = next
            } else {
                ranges.append(NSRange(location: NSNotFound, length: 0))
            }
        }

        return TextAlignmentResult(displayText: text, ranges: ranges)
    }

    private static func findLiteral(_ literal: String, in text: NSString, from: Int) -> NSRange? {
        guard !literal.isEmpty else { return nil }
        let tail = NSRange(location: from, length: max(0, text.length - from))
        let found = text.range(of: literal, options: [], range: tail)
        return found.location == NSNotFound ? nil : found
    }

    private static func findWord(_ normTarget: String, in text: NSString, from: Int) -> (NSRange, Int)? {
        var i = from
        while i < text.length {
            while i < text.length {
                let ch = text.character(at: i)
                if ch == 0x0A || ch == 0x0D {
                    return nil
                }
                if !CharacterSet.whitespaces.contains(UnicodeScalar(ch)!) {
                    break
                }
                i += 1
            }
            if i >= text.length { return nil }

            var j = i
            while j < text.length {
                let ch = text.character(at: j)
                if CharacterSet.whitespaces.contains(UnicodeScalar(ch)!) {
                    break
                }
                j += 1
            }

            let sliceRange = NSRange(location: i, length: j - i)
            let slice = text.substring(with: sliceRange)
            let normSlice = SpeechText.normalizeToken(slice)

            if normSlice == normTarget
                || normSlice.hasPrefix(normTarget)
                || normTarget.hasPrefix(normSlice)
            {
                let matchLen = matchedUTF16Length(in: text, start: i, normTarget: normTarget) ?? (j - i)
                let matched = NSRange(location: i, length: matchLen)
                return (matched, i + matchLen)
            }

            i = j
        }
        return nil
    }

    /// Length in UTF-16 code units of the source prefix that spells `normTarget`.
    private static func matchedUTF16Length(in text: NSString, start: Int, normTarget: String) -> Int? {
        var norm = ""
        var i = start
        while i < text.length {
            let ch = text.character(at: i)
            if let scalar = UnicodeScalar(ch), CharacterSet.alphanumerics.contains(scalar) {
                norm.append(Character(scalar).lowercased())
                if norm.count > normTarget.count { return nil }
                if norm == normTarget {
                    return i - start + 1
                }
                if !normTarget.hasPrefix(norm) {
                    return nil
                }
            }
            i += 1
        }
        return norm == normTarget ? i - start : nil
    }

    /// Fallback: display text built from Kokoro's own timestamp words (always in sync with audio).
    private static func buildFromTimestamps(_ timestamps: [WordTimestamp]) -> TextAlignmentResult {
        var display = ""
        var ranges: [NSRange] = []
        for ts in timestamps {
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
        }
        return TextAlignmentResult(displayText: display, ranges: ranges)
    }
}
