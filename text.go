package main

import (
	"strings"
	"unicode"
)

// normalizeForSpeech rewrites text so a TTS engine pronounces it naturally.
//
// Kokoro (like most OpenAI-compatible engines) treats an ALL-CAPS word as an
// acronym and spells it out letter by letter. Books use all caps for *emphasis*
// ("she was NEVER coming back"), which should be read as a normal word. We
// lower-case all-caps words that look like real words (contain a vowel), while
// leaving consonant-only acronyms (FBI, NBC) and single letters (I, A) alone so
// they are still spelled when that is actually intended.
func normalizeForSpeech(text string) string {
	var b strings.Builder
	b.Grow(len(text))

	word := make([]rune, 0, 16)
	flush := func() {
		if len(word) > 0 {
			b.WriteString(deEmphasize(string(word)))
			word = word[:0]
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || r == '\'' || r == '’' {
			word = append(word, r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}

// deEmphasize lower-cases an all-caps word that contains a vowel; otherwise it
// returns the word unchanged.
func deEmphasize(w string) string {
	letters := 0
	hasVowel := false
	for _, r := range w {
		if unicode.IsLetter(r) {
			letters++
			if !unicode.IsUpper(r) {
				return w // not all-caps
			}
			switch unicode.ToLower(r) {
			case 'a', 'e', 'i', 'o', 'u', 'y':
				hasVowel = true
			}
		}
	}
	// Require 4+ letters so short acronyms (US, UK, FBI, CIA, TV) stay spelled,
	// while multi-letter emphasis words (NEVER, REALLY, EVERYTHING) are spoken.
	if letters < 4 || !hasVowel {
		return w
	}
	return strings.ToLower(w)
}
