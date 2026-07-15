package main

import "testing"

func TestNormalizeForSpeechRewritesRomanListMarkers(t *testing.T) {
	input := "The clauses are (i) first; (ii) second; (iii) third; and (iv) fourth."
	want := "The clauses are one first; two second; three third; and four fourth."

	if got := normalizeForSpeech(input); got != want {
		t.Fatalf("normalizeForSpeech() = %q, want %q", got, want)
	}
}

func TestNormalizeForSpeechRewritesUppercaseAndHigherRomanMarkers(t *testing.T) {
	input := "(X) NEVER skip (XXXIX) the last item."
	want := "ten never skip thirty-nine the last item."

	if got := normalizeForSpeech(input); got != want {
		t.Fatalf("normalizeForSpeech() = %q, want %q", got, want)
	}
}

func TestNormalizeForSpeechLeavesNonMarkersAndInvalidRomans(t *testing.T) {
	input := "I saw ii plates; keep (i.e.), (in), (iiii), (ic), (vx), (xl), and (c) unchanged."

	if got := normalizeForSpeech(input); got != input {
		t.Fatalf("normalizeForSpeech() = %q, want unchanged input", got)
	}
}

func TestRomanListValueRequiresCanonicalFormWithinRange(t *testing.T) {
	tests := []struct {
		roman string
		value int
		ok    bool
	}{
		{"i", 1, true},
		{"IX", 9, true},
		{"xxxix", 39, true},
		{"iiii", 0, false},
		{"vx", 0, false},
		{"xl", 0, false},
	}

	for _, test := range tests {
		value, ok := romanListValue(test.roman)
		if value != test.value || ok != test.ok {
			t.Errorf("romanListValue(%q) = (%d, %v), want (%d, %v)",
				test.roman, value, ok, test.value, test.ok)
		}
	}
}
