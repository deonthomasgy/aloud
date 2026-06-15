import SwiftUI

struct AudioTimelineView: View {
    @Binding var current: TimeInterval
    let duration: TimeInterval
    let onSeek: (TimeInterval) -> Void
    let onScrubbingChanged: (Bool) -> Void

    var body: some View {
        VStack(spacing: 8) {
            Slider(
                value: $current,
                in: 0...max(duration, 0.01),
                onEditingChanged: { editing in
                    onScrubbingChanged(editing)
                    if !editing {
                        onSeek(current)
                    }
                }
            )
            .disabled(duration <= 0)

            HStack {
                Text(formatTime(current))
                Spacer()
                Text("−\(formatTime(max(0, duration - current)))")
                Spacer()
                Text(formatTime(duration))
            }
            .font(.caption.monospacedDigit())
            .foregroundStyle(.secondary)
        }
    }

    private func formatTime(_ seconds: TimeInterval) -> String {
        guard seconds.isFinite, seconds >= 0 else { return "0:00" }
        let total = Int(seconds.rounded(.down))
        let m = total / 60
        let s = total % 60
        return String(format: "%d:%02d", m, s)
    }
}
