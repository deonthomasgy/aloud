import Foundation

enum AudioFormat: String, CaseIterable, Identifiable {
    case mp3, wav, opus, flac, aac

    var id: String { rawValue }
    var label: String { rawValue.uppercased() }
}

struct CaptionedSpeechResult {
    let audio: Data
    let timestamps: [WordTimestamp]
}

struct KokoroClient {
    var baseURL: URL
    var apiKey: String = "not-needed"
    var model: String = "kokoro"

    private static let session: URLSession = {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 120
        config.timeoutIntervalForResource = 600
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()

    static let maxChars = 50_000

    /// Server root without /v1 — for /dev/* routes.
    var serverRoot: URL {
        var s = baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        if s.hasSuffix("/v1") {
            s = String(s.dropLast(3))
        }
        return URL(string: s) ?? baseURL.deletingLastPathComponent()
    }

    func checkHealth() async -> (ok: Bool, message: String) {
        let url = baseURL.appendingPathComponent("models")
        var req = URLRequest(url: url)
        req.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        req.timeoutInterval = 5

        do {
            let (_, resp) = try await Self.session.data(for: req)
            guard let http = resp as? HTTPURLResponse else {
                return (false, "Invalid response")
            }
            if http.statusCode == 200 {
                return (true, baseURL.absoluteString)
            }
            return (false, "Kokoro returned HTTP \(http.statusCode)")
        } catch {
            return (false, error.localizedDescription)
        }
    }

    func synthesize(text: String, voice: String, format: AudioFormat, speed: Double) async throws -> Data {
        let url = baseURL.appendingPathComponent("audio/speech")
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")

        let body: [String: Any] = [
            "model": model,
            "input": text,
            "voice": voice,
            "response_format": format.rawValue,
            "speed": speed,
        ]
        req.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (data, resp) = try await Self.session.data(for: req)
        guard let http = resp as? HTTPURLResponse else {
            throw KokoroError.badResponse
        }
        guard http.statusCode == 200 else {
            throw KokoroError.server(httpErrorMessage(data: data, status: http.statusCode))
        }
        return data
    }

    /// Synthesize with word timestamps; falls back to plain speech if captioned endpoint fails.
    func synthesizeWithTimestamps(
        text: String,
        voice: String,
        format: AudioFormat,
        speed: Double
    ) async throws -> CaptionedSpeechResult {
        do {
            return try await synthesizeCaptioned(text: text, voice: voice, format: format, speed: speed)
        } catch {
            let audio = try await synthesize(text: text, voice: voice, format: format, speed: speed)
            return CaptionedSpeechResult(audio: audio, timestamps: [])
        }
    }

    func synthesizeCaptioned(text: String, voice: String, format: AudioFormat, speed: Double) async throws -> CaptionedSpeechResult {
        let root = serverRoot.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard let url = URL(string: root + "/dev/captioned_speech") else {
            throw KokoroError.server("Invalid Kokoro URL")
        }

        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")

        let body: [String: Any] = [
            "model": model,
            "input": text,
            "voice": voice,
            "response_format": format.rawValue,
            "speed": speed,
            "stream": false,
            "normalization_options": ["normalize": false],
        ]
        req.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (data, resp) = try await Self.session.data(for: req)
        guard let http = resp as? HTTPURLResponse else {
            throw KokoroError.badResponse
        }
        guard http.statusCode == 200 else {
            throw KokoroError.server(httpErrorMessage(data: data, status: http.statusCode))
        }

        return try parseCaptionedResponse(data)
    }

    private func parseCaptionedResponse(_ data: Data) throws -> CaptionedSpeechResult {
        guard let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              let audioB64 = json["audio"] as? String,
              let audio = Data(base64Encoded: audioB64) else {
            throw KokoroError.server("Invalid captioned_speech response")
        }
        let rawTS = json["timestamps"] as? [[String: Any]] ?? []
        let timestamps = rawTS.enumerated().compactMap { WordTimestamp(id: $0.offset, json: $0.element) }
        return CaptionedSpeechResult(audio: audio, timestamps: timestamps)
    }

    private func httpErrorMessage(data: Data, status: Int) -> String {
        if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let err = json["error"] as? String ?? json["detail"] as? String {
            return err
        }
        let snippet = String(data: data.prefix(256), encoding: .utf8) ?? ""
        return "HTTP \(status): \(snippet)"
    }
}

enum KokoroError: LocalizedError {
    case badResponse
    case server(String)
    case cancelled

    var errorDescription: String? {
        switch self {
        case .badResponse: return "Unexpected response from Kokoro"
        case .server(let msg): return msg
        case .cancelled: return "Cancelled"
        }
    }
}
