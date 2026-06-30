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
        let start = Self.parseTime(json["start_time"] ?? json["start"])
        let endValue = json["end_time"] ?? json["end"]
        let end = endValue == nil ? start : Self.parseTime(endValue)
        self.id = id
        self.word = word
        self.startTime = start
        self.endTime = end
    }

    private static func parseTime(_ value: Any?) -> Double {
        switch value {
        case let n as Double: return n
        case let n as Float: return Double(n)
        case let n as Int: return Double(n)
        case let n as NSNumber: return n.doubleValue
        case let s as String: return Double(s) ?? 0
        default: return 0
        }
    }
}
