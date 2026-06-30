import Foundation
import NaturalLanguage

/// Semantic title + summary for AI-indexable audio filenames.
struct AudioTitle {
    let slug: String
    let summary: String
    let kind: String
}

struct TitleGeneratorConfig {
    var llmBaseURL: String
    var llmModel: String
    var llmAPIKey: String
    var useLLM: Bool

    static let defaults = TitleGeneratorConfig(
        llmBaseURL: "",
        llmModel: "llama3.2",
        llmAPIKey: "",
        useLLM: true
    )

    var llmURL: URL? {
        let trimmed = llmBaseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        var s = trimmed
        if !s.hasSuffix("/v1") {
            s = s.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + "/v1"
        }
        return URL(string: s)
    }
}

enum TitleGenerator {
  private static let session: URLSession = {
        let c = URLSessionConfiguration.default
        c.timeoutIntervalForRequest = 45
        return URLSession(configuration: c)
    }()

    static func generate(from text: String, config: TitleGeneratorConfig) async -> AudioTitle {
        let prepared = SpeechText.sanitize(text)
        let sample = textSample(from: prepared)

        if config.useLLM, let llmURL = config.llmURL,
           let ai = await requestLLMTitle(text: sample, config: config, baseURL: llmURL) {
            return ai
        }
        return analyzeOnDevice(text: prepared, sample: sample)
    }

    // MARK: - LLM

    private static func requestLLMTitle(
        text: String,
        config: TitleGeneratorConfig,
        baseURL: URL
    ) async -> AudioTitle? {
        let url = baseURL.appendingPathComponent("chat/completions")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if !config.llmAPIKey.isEmpty {
            req.setValue("Bearer \(config.llmAPIKey)", forHTTPHeaderField: "Authorization")
        }

        let prompt = """
        You name audio files for a speech library used by humans and AI systems.

        Read the script excerpt and respond with ONLY valid JSON (no markdown):
        {"title":"3-6 word topic slug using lowercase and hyphens","summary":"one sentence describing what this speech is about","kind":"one of: letter, story, article, instructions, dialogue, notes, speech, other"}

        Rules for title: describe the topic, not the first words. No file extension. No quotes inside values. Filesystem safe.
        """

        let body: [String: Any] = [
            "model": config.llmModel,
            "messages": [
                ["role": "system", "content": prompt],
                ["role": "user", "content": "Script (\(text.count) chars excerpt):\n\n\(text)"],
            ],
            "temperature": 0.3,
            "max_tokens": 120,
        ]

        guard let data = try? JSONSerialization.data(withJSONObject: body) else { return nil }

        do {
            req.httpBody = data
            let (respData, resp) = try await session.data(for: req)
            guard let http = resp as? HTTPURLResponse, http.statusCode == 200 else { return nil }
            return parseLLMResponse(respData)
        } catch {
            return nil
        }
    }

    private static func parseLLMResponse(_ data: Data) -> AudioTitle? {
        guard let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let choices = json["choices"] as? [[String: Any]],
              let message = choices.first?["message"] as? [String: Any],
              let content = message["content"] as? String
        else { return nil }

        let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
        let jsonText = extractJSONObject(from: trimmed)
        guard let jsonData = jsonText.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: jsonData) as? [String: Any],
              let titleRaw = obj["title"] as? String
        else { return nil }

        let slug = SpeechText.filenameSlug(from: titleRaw, maxWords: 10, maxLength: 64)
        guard !slug.isEmpty else { return nil }

        let summary = (obj["summary"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let kind = (obj["kind"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "other"
        return AudioTitle(
            slug: slug,
            summary: summary.isEmpty ? summaryFromSlug(slug, kind: kind) : summary,
            kind: kind.isEmpty ? "other" : kind
        )
    }

    private static func extractJSONObject(from text: String) -> String {
        if let start = text.firstIndex(of: "{"), let end = text.lastIndex(of: "}") {
            return String(text[start...end])
        }
        return text
    }

    // MARK: - On-device analysis

    private static func analyzeOnDevice(text: String, sample: String) -> AudioTitle {
        let language = detectLanguage(sample)
        let kind = detectKind(text: sample)

        if let structured = titleFromStructure(text: sample, kind: kind) {
            return structured
        }

        let keywords = extractKeywords(from: sample, language: language)
        if !keywords.isEmpty {
            let slug = keywords.prefix(5).joined(separator: "-")
            return AudioTitle(
                slug: slug,
                summary: summaryFromKeywords(keywords, kind: kind),
                kind: kind
            )
        }

        let slug = SpeechText.filenameSlug(from: sample)
        return AudioTitle(
            slug: slug.isEmpty ? "speech-clip" : slug,
            summary: firstSentence(from: sample, maxLength: 160),
            kind: kind
        )
    }

    private static func detectLanguage(_ text: String) -> NLLanguage? {
        let recognizer = NLLanguageRecognizer()
        recognizer.processString(text)
        return recognizer.dominantLanguage
    }

    private static func detectKind(text: String) -> String {
        let lower = text.lowercased()
        let firstLine = text
            .split(separator: "\n", maxSplits: 1, omittingEmptySubsequences: false)
            .first
            .map(String.init)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? text

        if firstLine.range(of: #"^dear\s+\w"#, options: .regularExpression) != nil {
            return "letter"
        }
        if lower.contains("step 1") || lower.contains("instructions") || lower.hasPrefix("how to ") {
            return "instructions"
        }
        if firstLine.range(of: #"^(chapter|section|part)\s+\d+"#, options: .regularExpression) != nil {
            return "article"
        }
        if text.contains("\"") && text.filter({ $0 == "\"" }).count >= 4 {
            return "dialogue"
        }
        if text.split(separator: "\n").count == 1, text.count < 280 {
            return "speech"
        }
        if text.split(separator: "\n").count > 6 {
            return "article"
        }
        return "other"
    }

    private static func titleFromStructure(text: String, kind: String) -> AudioTitle? {
        let lines = text
            .split(separator: "\n", omittingEmptySubsequences: true)
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }

        guard let first = lines.first else { return nil }

        if kind == "letter" {
            let pattern = #"dear\s+([^,]+)"#
            if let regex = try? NSRegularExpression(pattern: pattern, options: .caseInsensitive),
               let result = regex.firstMatch(in: first, range: NSRange(first.startIndex..., in: first)),
               result.numberOfRanges > 1,
               let nameRange = Range(result.range(at: 1), in: first) {
                let name = String(first[nameRange]).trimmingCharacters(in: .whitespacesAndNewlines)
                let nameSlug = SpeechText.filenameSlug(from: name, maxWords: 3, maxLength: 24)
                if !nameSlug.isEmpty {
                    let topic = extractKeywords(from: text, language: nil).prefix(3).joined(separator: "-")
                    let slug = topic.isEmpty ? "letter-to-\(nameSlug)" : "letter-to-\(nameSlug)-\(topic)"
                    return AudioTitle(
                        slug: slug,
                        summary: "Personal letter addressed to \(name).",
                        kind: kind
                    )
                }
            }
        }

        if let heading = lines.first(where: { looksLikeHeading($0) }) {
            let slug = SpeechText.filenameSlug(from: heading, maxWords: 8, maxLength: 56)
            if !slug.isEmpty {
                return AudioTitle(
                    slug: slug,
                    summary: "Speech titled: \(heading)",
                    kind: kind
                )
            }
        }

        let sentence = firstSentence(from: text, maxLength: 100)
        if sentence.count >= 12, sentence.count <= 80,
           !sentence.hasSuffix(","),
           sentence.filter({ $0 == "." }).count <= 1 {
            let slug = SpeechText.filenameSlug(from: sentence, maxWords: 8, maxLength: 56)
            if slug.count >= 8 {
                return AudioTitle(
                    slug: slug,
                    summary: sentence,
                    kind: kind
                )
            }
        }

        return nil
    }

    private static func looksLikeHeading(_ line: String) -> Bool {
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.count >= 4, trimmed.count <= 72 else { return false }
        if trimmed.hasSuffix(".") && trimmed.filter({ $0 == "." }).count == 1 && trimmed.count > 40 {
            return false
        }
        if trimmed.range(of: #"^(chapter|section|part)\s+\d+"#, options: .regularExpression) != nil {
            return true
        }
        if trimmed == trimmed.uppercased(), trimmed.filter(\.isLetter).count >= 4 {
            return true
        }
        if trimmed.range(of: #"^\d+[\.\)]\s+\S"#, options: .regularExpression) != nil {
            return true
        }
        return trimmed.count <= 48 && !trimmed.contains("?") && trimmed.split(separator: " ").count <= 10
    }

    private static func extractKeywords(from text: String, language: NLLanguage?) -> [String] {
        let tagger = NLTagger(tagSchemes: [.lexicalClass, .nameType])
        tagger.string = text
        if let language {
            tagger.setLanguage(language, range: text.startIndex..<text.endIndex)
        }

        var scores: [String: Int] = [:]
        let options: NLTagger.Options = [.omitWhitespace, .omitPunctuation, .joinNames]

        tagger.enumerateTags(in: text.startIndex..<text.endIndex, unit: .word, scheme: .nameType, options: options) { tag, range in
            guard tag != nil else { return true }
            let word = String(text[range]).lowercased()
            guard word.count > 2 else { return true }
            scores[word, default: 0] += 4
            return true
        }

        tagger.enumerateTags(in: text.startIndex..<text.endIndex, unit: .word, scheme: .lexicalClass, options: options) { tag, range in
            guard tag == .noun else { return true }
            let word = String(text[range]).lowercased()
            guard word.count > 3, !stopWords.contains(word) else { return true }
            scores[word, default: 0] += 1
            return true
        }

        return scores
            .sorted { $0.value > $1.value }
            .map(\.key)
            .filter { !stopWords.contains($0) }
    }

    private static let stopWords: Set<String> = [
        "that", "this", "with", "from", "have", "were", "been", "their", "there",
        "would", "could", "should", "about", "which", "when", "what", "your",
        "they", "them", "then", "than", "into", "also", "just", "very", "some",
        "will", "shall", "such", "only", "other", "more", "most", "much",
    ]

    private static func textSample(from text: String, maxChars: Int = 2_000) -> String {
        guard text.count > maxChars else { return text }
        return String(text.prefix(maxChars)) + "…"
    }

    private static func firstSentence(from text: String, maxLength: Int) -> String {
        let tokenizer = NLTokenizer(unit: .sentence)
        tokenizer.string = text
        var result = ""
        tokenizer.enumerateTokens(in: text.startIndex..<text.endIndex) { range, _ in
            result = String(text[range]).trimmingCharacters(in: .whitespacesAndNewlines)
            return false
        }
        if result.count > maxLength {
            result = String(result.prefix(maxLength)).trimmingCharacters(in: .whitespaces) + "…"
        }
        return result
    }

    private static func summaryFromKeywords(_ keywords: [String], kind: String) -> String {
        let topic = keywords.prefix(4).joined(separator: ", ")
        switch kind {
        case "letter": return "Letter about \(topic)."
        case "instructions": return "Instructions covering \(topic)."
        case "dialogue": return "Dialogue involving \(topic)."
        case "article": return "Article discussing \(topic)."
        default: return "Speech about \(topic)."
        }
    }

    private static func summaryFromSlug(_ slug: String, kind: String) -> String {
        let words = slug.replacingOccurrences(of: "-", with: " ")
        switch kind {
        case "letter": return "Letter: \(words)."
        case "instructions": return "Instructions: \(words)."
        default: return "Speech: \(words)."
        }
    }

    /// Derive a likely Ollama URL from the Kokoro host (same machine, port 11434).
    static func suggestedLLMURL(fromKokoro baseURLString: String) -> String {
        guard let url = URL(string: baseURLString.trimmingCharacters(in: .whitespacesAndNewlines)),
              let host = url.host else {
            return "http://localhost:11434/v1"
        }
        return "http://\(host):11434/v1"
    }
}

/// Sidecar metadata written alongside saved audio for AI tooling.
struct AudioMetadata: Codable {
    let title: String
    let summary: String
    let kind: String
    let voice: String
    let format: String
    let language: String?
    let sourceCharCount: Int
    let generatedAt: Date

    func write(alongside audioURL: URL) throws {
        let metaURL = audioURL.deletingPathExtension().appendingPathExtension("invtts.json")
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        try encoder.encode(self).write(to: metaURL, options: .atomic)
    }
}
