package main

import "testing"

func TestDefaultVoiceIsValid(t *testing.T) {
	if !isValidVoice(DefaultVoice) {
		t.Fatalf("default voice %q is not in catalogue", DefaultVoice)
	}
}

func TestIsValidVoice(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"af_heart", true},
		{"am_adam", true},
		{"bf_alice", true},
		{"jf_alpha", true},
		{"not_a_voice", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isValidVoice(tc.id); got != tc.valid {
			t.Errorf("isValidVoice(%q) = %v, want %v", tc.id, got, tc.valid)
		}
	}
}

func TestVoiceCatalogueNotEmpty(t *testing.T) {
	count := 0
	for _, g := range VoiceGroups {
		if g.Language == "" {
			t.Fatal("voice group missing language label")
		}
		count += len(g.Voices)
	}
	if count < 50 {
		t.Fatalf("expected a large voice catalogue, got %d voices", count)
	}
}
