package main

// Voice is a single Kokoro voice option exposed in the UI.
type Voice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VoiceGroup groups voices by language, mirroring the kokorottsai.com selector.
type VoiceGroup struct {
	Language string  `json:"language"`
	Voices   []Voice `json:"voices"`
}

// DefaultVoice is the voice selected on first load.
const DefaultVoice = "af_heart"

// VoiceGroups is the full Kokoro-82M voice catalogue, grouped by language.
// IDs follow Kokoro's convention: <lang><gender>_<name> (e.g. af_heart = American female "heart").
var VoiceGroups = []VoiceGroup{
	{
		Language: "American English",
		Voices: []Voice{
			{"af_heart", "Heart (F)"},
			{"af_alloy", "Alloy (F)"},
			{"af_aoede", "Aoede (F)"},
			{"af_bella", "Bella (F)"},
			{"af_jessica", "Jessica (F)"},
			{"af_kore", "Kore (F)"},
			{"af_nicole", "Nicole (F)"},
			{"af_nova", "Nova (F)"},
			{"af_river", "River (F)"},
			{"af_sarah", "Sarah (F)"},
			{"af_sky", "Sky (F)"},
			{"am_adam", "Adam (M)"},
			{"am_echo", "Echo (M)"},
			{"am_eric", "Eric (M)"},
			{"am_fenrir", "Fenrir (M)"},
			{"am_liam", "Liam (M)"},
			{"am_michael", "Michael (M)"},
			{"am_onyx", "Onyx (M)"},
			{"am_puck", "Puck (M)"},
			{"am_santa", "Santa (M)"},
		},
	},
	{
		Language: "British English",
		Voices: []Voice{
			{"bf_alice", "Alice (F)"},
			{"bf_emma", "Emma (F)"},
			{"bf_isabella", "Isabella (F)"},
			{"bf_lily", "Lily (F)"},
			{"bm_daniel", "Daniel (M)"},
			{"bm_fable", "Fable (M)"},
			{"bm_george", "George (M)"},
			{"bm_lewis", "Lewis (M)"},
		},
	},
	{
		Language: "Japanese",
		Voices: []Voice{
			{"jf_alpha", "Alpha (F)"},
			{"jf_gongitsune", "Gongitsune (F)"},
			{"jf_nezumi", "Nezumi (F)"},
			{"jf_tebukuro", "Tebukuro (F)"},
			{"jm_kumo", "Kumo (M)"},
		},
	},
	{
		Language: "Mandarin Chinese",
		Voices: []Voice{
			{"zf_xiaobei", "Xiaobei (F)"},
			{"zf_xiaoni", "Xiaoni (F)"},
			{"zf_xiaoxiao", "Xiaoxiao (F)"},
			{"zf_xiaoyi", "Xiaoyi (F)"},
			{"zm_yunjian", "Yunjian (M)"},
			{"zm_yunxi", "Yunxi (M)"},
			{"zm_yunxia", "Yunxia (M)"},
			{"zm_yunyang", "Yunyang (M)"},
		},
	},
	{
		Language: "French",
		Voices: []Voice{
			{"ff_siwis", "Siwis (F)"},
		},
	},
	{
		Language: "Hindi",
		Voices: []Voice{
			{"hf_alpha", "Alpha (F)"},
			{"hf_beta", "Beta (F)"},
			{"hm_omega", "Omega (M)"},
			{"hm_psi", "Psi (M)"},
		},
	},
	{
		Language: "Italian",
		Voices: []Voice{
			{"if_sara", "Sara (F)"},
			{"im_nicola", "Nicola (M)"},
		},
	},
	{
		Language: "Brazilian Portuguese",
		Voices: []Voice{
			{"pf_dora", "Dora (F)"},
			{"pm_alex", "Alex (M)"},
			{"pm_santa", "Santa (M)"},
		},
	},
	{
		Language: "Spanish",
		Voices: []Voice{
			{"ef_dora", "Dora (F)"},
			{"em_alex", "Alex (M)"},
			{"em_santa", "Santa (M)"},
		},
	},
}

// validVoices is a fast lookup set built from VoiceGroups.
var validVoices = func() map[string]bool {
	m := make(map[string]bool)
	for _, g := range VoiceGroups {
		for _, v := range g.Voices {
			m[v.ID] = true
		}
	}
	return m
}()

// isValidVoice reports whether id is a known Kokoro voice.
func isValidVoice(id string) bool { return validVoices[id] }
