import AVFoundation
import AppKit
import Foundation
import UniformTypeIdentifiers

@MainActor
final class TTSViewModel: NSObject, ObservableObject {
    @Published var text = ""
    @Published var selectedLanguage = "American English"
    @Published var selectedVoice = VoiceCatalog.defaultVoice
    @Published var format: AudioFormat = .mp3
    @Published var speed: Double = 1.0

    @Published var isGenerating = false
    @Published var statusMessage = ""
    @Published var isError = false
    @Published var kokoroConnected = false
    @Published var isPlaying = false
    @Published var playbackTime: TimeInterval = 0
    @Published var playbackDuration: TimeInterval = 0
    @Published var isScrubbing = false
    @Published var activeWordIndex: Int?
    @Published var wordRanges: [NSRange] = []
    @Published var karaokeEnabled = false
    @Published private(set) var spokenText = ""

    @Published var baseURLString: String {
        didSet { UserDefaults.standard.set(baseURLString, forKey: "kokoroBaseURL") }
    }

    @Published var apiKey: String {
        didSet { UserDefaults.standard.set(apiKey, forKey: "kokoroAPIKey") }
    }

    private var audioPlayer: AVAudioPlayer?
    private var audioData: Data?
    private var wordTimestamps: [WordTimestamp] = []
    private var highlightTimer: Timer?
    private var progressTimer: Timer?
    private var generateTask: Task<Void, Never>?

    /// Karaoke disabled for very long passages (UI + alignment cost).
    static let karaokeMaxWords = 400
    static let karaokeMaxChars = 8_000

    /// Show karaoke only while audio is actually playing.
    var showKaraoke: Bool {
        karaokeEnabled && isPlaying && !spokenText.isEmpty
    }

    override init() {
        baseURLString = UserDefaults.standard.string(forKey: "kokoroBaseURL")
            ?? "http://alpha-old:8880/v1"
        apiKey = UserDefaults.standard.string(forKey: "kokoroAPIKey") ?? "not-needed"
        super.init()
        Task { await refreshHealth() }
    }

    private func makeClient() throws -> KokoroClient {
        var urlString = baseURLString.trimmingCharacters(in: .whitespacesAndNewlines)
        if !urlString.hasSuffix("/v1") {
            urlString = urlString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + "/v1"
        }
        guard let url = URL(string: urlString) else {
            throw KokoroError.server("Invalid Kokoro URL")
        }
        return KokoroClient(baseURL: url, apiKey: apiKey)
    }

    func refreshHealth() async {
        do {
            let client = try makeClient()
            let result = await client.checkHealth()
            kokoroConnected = result.ok
            if result.ok {
                statusMessage = "Connected · \(result.message)"
                isError = false
            } else if statusMessage.isEmpty || !isGenerating {
                statusMessage = result.message
                isError = true
            }
        } catch {
            kokoroConnected = false
            statusMessage = error.localizedDescription
            isError = true
        }
    }

    func generate() {
        generateTask?.cancel()
        generateTask = Task { await runGenerate() }
    }

    func cancelGenerate() {
        generateTask?.cancel()
        generateTask = nil
        stopProgressTimer()
        isGenerating = false
        statusMessage = "Cancelled"
        isError = false
    }

    private func runGenerate() async {
        let cleaned = SpeechText.sanitize(text)
        guard !cleaned.isEmpty else {
            statusMessage = "Enter some text first."
            isError = true
            return
        }
        if cleaned.count > KokoroClient.maxChars {
            statusMessage = "Text too long (\(cleaned.count) chars, max \(KokoroClient.maxChars))"
            isError = true
            return
        }

        isGenerating = true
        isError = false
        statusMessage = "Synthesizing…"
        stopPlayback(clearKaraoke: true)
        startProgressTimer()

        defer {
            stopProgressTimer()
            isGenerating = false
            generateTask = nil
        }

        let started = Date()
        let voice = selectedVoice
        let outputFormat = format
        let outputSpeed = speed
        let client: KokoroClient
        do {
            client = try makeClient()
        } catch {
            statusMessage = error.localizedDescription
            isError = true
            return
        }

        do {
            let result = try await Task.detached(priority: .userInitiated) {
                try await client.synthesizeWithTimestamps(
                    text: cleaned,
                    voice: voice,
                    format: outputFormat,
                    speed: outputSpeed
                )
            }.value

            try Task.checkCancellation()

            audioData = result.audio
            wordTimestamps = result.timestamps

            let canKaraoke = !result.timestamps.isEmpty
                && result.timestamps.count <= Self.karaokeMaxWords
                && cleaned.count <= Self.karaokeMaxChars

            if canKaraoke {
                let alignment = await Task.detached(priority: .userInitiated) {
                    TextAlignment.align(text: cleaned, timestamps: result.timestamps)
                }.value
                spokenText = alignment.displayText
                wordRanges = alignment.ranges
            } else {
                spokenText = cleaned
                wordRanges = []
            }

            karaokeEnabled = canKaraoke
            activeWordIndex = nil

            // Play first — enable karaoke after the UI settles.
            try startPlayback(data: result.audio)
            if canKaraoke {
                activeWordIndex = 0
            }

            let elapsed = Date().timeIntervalSince(started)
            let kb = Double(result.audio.count) / 1024.0
            var note = canKaraoke ? " · word sync" : ""
            if !canKaraoke, !result.timestamps.isEmpty {
                note = " · word sync off (text too long)"
            } else if result.timestamps.isEmpty, !cleaned.isEmpty {
                note += " · no timestamps"
            }
            statusMessage = String(format: "Done in %.1fs · %.0f KB%@", elapsed, kb, note)
            isError = false
            kokoroConnected = true
        } catch is CancellationError {
            statusMessage = "Cancelled"
            isError = false
        } catch {
            if Task.isCancelled {
                statusMessage = "Cancelled"
                isError = false
            } else {
                statusMessage = error.localizedDescription
                isError = true
                await refreshHealth()
            }
        }
    }

    private func startProgressTimer() {
        stopProgressTimer()
        let t0 = Date()
        progressTimer = Timer(timeInterval: 1.0, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self, self.isGenerating else { return }
                let s = Int(Date().timeIntervalSince(t0))
                self.statusMessage = "Synthesizing… \(s)s"
            }
        }
        RunLoop.main.add(progressTimer!, forMode: .common)
    }

    private func stopProgressTimer() {
        progressTimer?.invalidate()
        progressTimer = nil
    }

    func importFromWord(mode: WordImporter.ImportMode) {
        do {
            let imported = try WordImporter.importText(mode: mode)
            text = imported
            stopPlayback(clearKaraoke: true)
            let label = mode == .selection ? "selection" : "document"
            statusMessage = "Imported Word \(label) · \(imported.count) chars"
            isError = false
            NSApp.activate(ignoringOtherApps: true)
        } catch {
            statusMessage = error.localizedDescription
            isError = true
        }
    }

    func saveAudio() {
        guard let data = audioData else { return }
        let panel = NSSavePanel()
        panel.allowedContentTypes = [format.utType]
        panel.nameFieldStringValue = "speech-\(selectedVoice).\(format.rawValue)"
        panel.begin { response in
            guard response == .OK, let url = panel.url else { return }
            try? data.write(to: url)
        }
    }

    func seek(to time: TimeInterval) {
        guard let player = audioPlayer else { return }
        let clamped = min(max(0, time), player.duration)
        player.currentTime = clamped
        playbackTime = clamped
        syncWordHighlight(at: clamped)
    }

    func setScrubbing(_ scrubbing: Bool) {
        isScrubbing = scrubbing
    }

    func togglePlayback() {
        if isPlaying {
            audioPlayer?.pause()
            isPlaying = false
            stopHighlightTimer()
        } else if let player = audioPlayer {
            player.play()
            isPlaying = true
            startHighlightTimer()
        } else if let data = audioData {
            try? startPlayback(data: data)
        }
    }

    func stopPlayback(clearKaraoke: Bool = false) {
        audioPlayer?.stop()
        audioPlayer = nil
        isPlaying = false
        stopHighlightTimer()
        activeWordIndex = nil
        playbackTime = 0
        playbackDuration = 0
        isScrubbing = false
        if clearKaraoke {
            wordTimestamps = []
            wordRanges = []
            karaokeEnabled = false
            spokenText = ""
        }
    }

    private func startPlayback(data: Data) throws {
        audioPlayer = try AVAudioPlayer(data: data)
        audioPlayer?.delegate = self
        audioPlayer?.prepareToPlay()
        playbackDuration = audioPlayer?.duration ?? 0
        playbackTime = 0
        guard audioPlayer?.play() == true else {
            throw KokoroError.server("Audio playback failed to start")
        }
        isPlaying = true
        startHighlightTimer()
    }

    private func startHighlightTimer() {
        stopHighlightTimer()
        let timer = Timer(timeInterval: 1.0 / 30.0, repeats: true) { [weak self] _ in
            Task { @MainActor in
                self?.updatePlaybackTick()
            }
        }
        RunLoop.main.add(timer, forMode: .common)
        highlightTimer = timer
    }

    private func stopHighlightTimer() {
        highlightTimer?.invalidate()
        highlightTimer = nil
    }

    private func updatePlaybackTick() {
        guard let player = audioPlayer else { return }
        playbackDuration = player.duration

        if !isScrubbing {
            playbackTime = player.currentTime
        }

        if player.isPlaying {
            syncWordHighlight(at: player.currentTime)
        }
    }

    private func syncWordHighlight(at time: TimeInterval) {
        guard karaokeEnabled, isPlaying, !wordTimestamps.isEmpty else { return }

        let idx: Int?
        if let found = wordTimestamps.firstIndex(where: { time >= $0.startTime - 0.02 && time < $0.endTime }) {
            idx = found
        } else if time >= (wordTimestamps.last?.endTime ?? 0) {
            idx = nil
        } else {
            return
        }

        guard idx != activeWordIndex else { return }
        guard idx == nil || (idx! < wordRanges.count && wordRanges[idx!].location != NSNotFound) else { return }
        activeWordIndex = idx
    }

    var hasAudio: Bool { audioData != nil }

    var charCount: Int { text.count }
}

extension TTSViewModel: AVAudioPlayerDelegate {
    nonisolated func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
        Task { @MainActor in
            isPlaying = false
            stopHighlightTimer()
            activeWordIndex = nil
            karaokeEnabled = false
            playbackTime = playbackDuration
        }
    }
}

private extension AudioFormat {
    var utType: UTType {
        switch self {
        case .mp3: return .mp3
        case .wav: return .wav
        case .opus: return UTType(filenameExtension: "opus") ?? .audio
        case .flac: return UTType(filenameExtension: "flac") ?? .audio
        case .aac: return UTType(filenameExtension: "aac") ?? .audio
        }
    }
}
