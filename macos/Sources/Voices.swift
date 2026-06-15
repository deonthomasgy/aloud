import Foundation

struct VoiceOption: Identifiable, Hashable {
    let id: String
    let name: String
    let language: String
}

enum VoiceCatalog {
    static let defaultVoice = "af_heart"

    static let all: [VoiceOption] = [
        // American English
        .init(id: "af_heart", name: "Heart (F)", language: "American English"),
        .init(id: "af_alloy", name: "Alloy (F)", language: "American English"),
        .init(id: "af_aoede", name: "Aoede (F)", language: "American English"),
        .init(id: "af_bella", name: "Bella (F)", language: "American English"),
        .init(id: "af_jessica", name: "Jessica (F)", language: "American English"),
        .init(id: "af_kore", name: "Kore (F)", language: "American English"),
        .init(id: "af_nicole", name: "Nicole (F)", language: "American English"),
        .init(id: "af_nova", name: "Nova (F)", language: "American English"),
        .init(id: "af_river", name: "River (F)", language: "American English"),
        .init(id: "af_sarah", name: "Sarah (F)", language: "American English"),
        .init(id: "af_sky", name: "Sky (F)", language: "American English"),
        .init(id: "am_adam", name: "Adam (M)", language: "American English"),
        .init(id: "am_echo", name: "Echo (M)", language: "American English"),
        .init(id: "am_eric", name: "Eric (M)", language: "American English"),
        .init(id: "am_fenrir", name: "Fenrir (M)", language: "American English"),
        .init(id: "am_liam", name: "Liam (M)", language: "American English"),
        .init(id: "am_michael", name: "Michael (M)", language: "American English"),
        .init(id: "am_onyx", name: "Onyx (M)", language: "American English"),
        .init(id: "am_puck", name: "Puck (M)", language: "American English"),
        .init(id: "am_santa", name: "Santa (M)", language: "American English"),
        // British English
        .init(id: "bf_alice", name: "Alice (F)", language: "British English"),
        .init(id: "bf_emma", name: "Emma (F)", language: "British English"),
        .init(id: "bf_isabella", name: "Isabella (F)", language: "British English"),
        .init(id: "bf_lily", name: "Lily (F)", language: "British English"),
        .init(id: "bm_daniel", name: "Daniel (M)", language: "British English"),
        .init(id: "bm_fable", name: "Fable (M)", language: "British English"),
        .init(id: "bm_george", name: "George (M)", language: "British English"),
        .init(id: "bm_lewis", name: "Lewis (M)", language: "British English"),
        // Japanese
        .init(id: "jf_alpha", name: "Alpha (F)", language: "Japanese"),
        .init(id: "jf_gongitsune", name: "Gongitsune (F)", language: "Japanese"),
        .init(id: "jf_nezumi", name: "Nezumi (F)", language: "Japanese"),
        .init(id: "jf_tebukuro", name: "Tebukuro (F)", language: "Japanese"),
        .init(id: "jm_kumo", name: "Kumo (M)", language: "Japanese"),
        // Mandarin Chinese
        .init(id: "zf_xiaobei", name: "Xiaobei (F)", language: "Mandarin Chinese"),
        .init(id: "zf_xiaoni", name: "Xiaoni (F)", language: "Mandarin Chinese"),
        .init(id: "zf_xiaoxiao", name: "Xiaoxiao (F)", language: "Mandarin Chinese"),
        .init(id: "zf_xiaoyi", name: "Xiaoyi (F)", language: "Mandarin Chinese"),
        .init(id: "zm_yunjian", name: "Yunjian (M)", language: "Mandarin Chinese"),
        .init(id: "zm_yunxi", name: "Yunxi (M)", language: "Mandarin Chinese"),
        .init(id: "zm_yunxia", name: "Yunxia (M)", language: "Mandarin Chinese"),
        .init(id: "zm_yunyang", name: "Yunyang (M)", language: "Mandarin Chinese"),
        // French
        .init(id: "ff_siwis", name: "Siwis (F)", language: "French"),
        // Hindi
        .init(id: "hf_alpha", name: "Alpha (F)", language: "Hindi"),
        .init(id: "hf_beta", name: "Beta (F)", language: "Hindi"),
        .init(id: "hm_omega", name: "Omega (M)", language: "Hindi"),
        .init(id: "hm_psi", name: "Psi (M)", language: "Hindi"),
        // Italian
        .init(id: "if_sara", name: "Sara (F)", language: "Italian"),
        .init(id: "im_nicola", name: "Nicola (M)", language: "Italian"),
        // Brazilian Portuguese
        .init(id: "pf_dora", name: "Dora (F)", language: "Brazilian Portuguese"),
        .init(id: "pm_alex", name: "Alex (M)", language: "Brazilian Portuguese"),
        .init(id: "pm_santa", name: "Santa (M)", language: "Brazilian Portuguese"),
        // Spanish
        .init(id: "ef_dora", name: "Dora (F)", language: "Spanish"),
        .init(id: "em_alex", name: "Alex (M)", language: "Spanish"),
        .init(id: "em_santa", name: "Santa (M)", language: "Spanish"),
    ]

    static var languages: [String] {
        Array(Set(all.map(\.language))).sorted()
    }

    static func voices(for language: String) -> [VoiceOption] {
        all.filter { $0.language == language }
    }
}
