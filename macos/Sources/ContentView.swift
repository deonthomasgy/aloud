import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var model: TTSViewModel

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            Form {
                textSection
                controlsSection
                statusSection
                if model.hasAudio {
                    playerSection
                }
            }
            .formStyle(.grouped)
            .padding(.bottom, 8)
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .toolbar { toolbarContent }
        .onChange(of: model.selectedLanguage) { lang in
            let voices = VoiceCatalog.voices(for: lang)
            if !voices.contains(where: { $0.id == model.selectedVoice }),
               let first = voices.first {
                model.selectedVoice = first.id
            }
        }
        .task {
            await model.refreshHealth()
        }
    }

    private var header: some View {
        HStack(spacing: 12) {
            Image(systemName: "waveform.circle.fill")
                .font(.system(size: 28))
                .symbolRenderingMode(.palette)
                .foregroundStyle(.white, .purple)
            VStack(alignment: .leading, spacing: 2) {
                Text("invtts")
                    .font(.title2.weight(.semibold))
                Text("Kokoro text-to-speech")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            connectionBadge
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 14)
    }

    private var connectionBadge: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(model.kokoroConnected ? Color.green : Color.red)
                .frame(width: 8, height: 8)
            Text(model.kokoroConnected ? "Kokoro online" : "Kokoro offline")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private var textSection: some View {
        Section {
            ZStack(alignment: .topLeading) {
                TextEditor(text: $model.text)
                    .font(.body)
                    .frame(minHeight: 160)
                    .disabled(model.showKaraoke)
                    .opacity(model.showKaraoke ? 0 : 1)
                    .overlay(alignment: .topLeading) {
                        if model.text.isEmpty, !model.showKaraoke {
                            Text("Paste or type text to turn into speech…")
                                .foregroundStyle(.tertiary)
                                .padding(.top, 8)
                                .padding(.leading, 8)
                                .allowsHitTesting(false)
                        }
                    }

                if model.showKaraoke {
                    KaraokeTextView(
                        text: model.spokenText,
                        wordRanges: model.wordRanges,
                        activeWordIndex: model.activeWordIndex
                    )
                    .frame(minHeight: 160)
                    .zIndex(1)
                }
            }
        } header: {
            HStack {
                Text("Your text")
                Spacer()
                Menu {
                    Button("Import Word Selection") {
                        model.importFromWord(mode: .selection)
                    }
                    Button("Import Word Document") {
                        model.importFromWord(mode: .activeDocument)
                    }
                } label: {
                    Label("Word", systemImage: "doc.text")
                }
                .menuStyle(.borderlessButton)
                .disabled(!WordImporter.isInstalled)
                .help(WordImporter.isInstalled
                      ? "Import text from Microsoft Word"
                      : "Microsoft Word not installed")
                if model.showKaraoke {
                    Label("Syncing", systemImage: "text.word.spacing")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Text("\(model.charCount.formatted()) characters")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var controlsSection: some View {
        Section("Voice & output") {
            Picker("Language", selection: $model.selectedLanguage) {
                ForEach(VoiceCatalog.languages, id: \.self) { lang in
                    Text(lang).tag(lang)
                }
            }
            Picker("Voice", selection: $model.selectedVoice) {
                ForEach(VoiceCatalog.voices(for: model.selectedLanguage)) { voice in
                    Text("\(voice.name) — \(voice.id)").tag(voice.id)
                }
            }
            Picker("Format", selection: $model.format) {
                ForEach(AudioFormat.allCases) { fmt in
                    Text(fmt.label).tag(fmt)
                }
            }
            HStack {
                Text("Speed")
                Slider(value: $model.speed, in: 0.5...2.0, step: 0.05)
                Text(String(format: "%.2f×", model.speed))
                    .monospacedDigit()
                    .frame(width: 48, alignment: .trailing)
            }
        }
    }

    private var statusSection: some View {
        Section {
            if !model.statusMessage.isEmpty {
                Label {
                    Text(model.statusMessage)
                        .foregroundStyle(model.isError ? .red : .secondary)
                } icon: {
                    Image(systemName: model.isError ? "exclamationmark.triangle.fill" : "info.circle")
                        .foregroundStyle(model.isError ? .red : .secondary)
                }
            }
        }
    }

    private var playerSection: some View {
        Section("Playback") {
            AudioTimelineView(
                current: $model.playbackTime,
                duration: model.playbackDuration,
                onSeek: { model.seek(to: $0) },
                onScrubbingChanged: { model.setScrubbing($0) }
            )

            HStack {
                Button {
                    model.togglePlayback()
                } label: {
                    Label(model.isPlaying ? "Pause" : "Play", systemImage: model.isPlaying ? "pause.fill" : "play.fill")
                }
                Button("Save…") {
                    model.saveAudio()
                }
            }

            if !model.suggestedAudioSlug.isEmpty || model.isDerivingTitle {
                HStack(spacing: 6) {
                    if model.isDerivingTitle {
                        ProgressView()
                            .controlSize(.small)
                        Text("Deriving title…")
                    } else {
                        Image(systemName: "tag.fill")
                            .foregroundStyle(.secondary)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(model.suggestedAudioSlug)
                                .font(.caption.weight(.medium))
                            if !model.suggestedAudioSummary.isEmpty {
                                Text(model.suggestedAudioSummary)
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(2)
                            }
                        }
                    }
                }
            }
        }
    }

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        ToolbarItem(placement: .primaryAction) {
            if model.isGenerating {
                Button {
                    model.cancelGenerate()
                } label: {
                    Label("Cancel", systemImage: "xmark.circle")
                }
            } else {
                Button {
                    model.generate()
                } label: {
                    Label("Generate", systemImage: "speaker.wave.2.fill")
                }
                .keyboardShortcut(.return, modifiers: .command)
                .disabled(model.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        ToolbarItem(placement: .automatic) {
            if model.isGenerating {
                ProgressView()
                    .controlSize(.small)
            }
        }
        ToolbarItem(placement: .automatic) {
            Button {
                model.text = ""
                model.stopPlayback(clearKaraoke: true)
            } label: {
                Label("Clear", systemImage: "xmark.circle")
            }
            .disabled(model.text.isEmpty)
        }
    }
}

struct SettingsView: View {
    @EnvironmentObject private var model: TTSViewModel

    var body: some View {
        Form {
            Section("Kokoro endpoint") {
                TextField("Base URL", text: $model.baseURLString, prompt: Text("http://alpha-old:8880/v1"))
                TextField("API key", text: $model.apiKey)
                Button("Test connection") {
                    Task { await model.refreshHealth() }
                }
            }
            Section {
                Text("Default: Kokoro on alpha-old over Tailscale. Change if your endpoint is elsewhere.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            Section("Audio titles") {
                Toggle("Use LLM for titles", isOn: $model.titleUseLLM)
                TextField("LLM API URL", text: $model.titleLLMBaseURL, prompt: Text("http://host:11434/v1"))
                TextField("Model", text: $model.titleLLMModel, prompt: Text("llama3.2"))
                TextField("API key (optional)", text: $model.titleLLMAPIKey)
                Button("Use Ollama on Kokoro host") {
                    model.applySuggestedLLMURL()
                }
            }
            Section {
                Text("Titles are derived from speech content for AI-friendly filenames. With no LLM configured, on-device language analysis is used. Saving also writes a .invtts.json sidecar.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 420)
        .padding()
    }
}
