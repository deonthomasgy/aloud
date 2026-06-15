import Foundation

struct WordTimestamp: Identifiable, Equatable {
    let id: Int
    let word: String
    let startTime: Double
    let endTime: Double

    init(id: Int, word: String, startTime: Double, endTime: Double) {
        self.id = id
        self.word = word
        self.startTime = startTime
        self.endTime = endTime
    }

    init?(id: Int, json: [String: Any]) {
        guard let word = json["word"] as? String else { return nil }
        let start = (json["start_time"] as? Double) ?? (json["start"] as? Double) ?? 0
        let end = (json["end_time"] as? Double) ?? (json["end"] as? Double) ?? start
        self.id = id
        self.word = word
        self.startTime = start
        self.endTime = end
    }
}
