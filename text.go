package main

import (
	"regexp"
	"strings"
	"unicode"
)

var parenthesizedRomanMarker = regexp.MustCompile(`\(([iIvVxX]+)\)`)

// normalizeForSpeech rewrites text so a TTS engine pronounces it naturally.
//
// Kokoro (like most OpenAI-compatible engines) treats an ALL-CAPS word as an
// acronym and spells it out letter by letter. Books use all caps for *emphasis*
// ("she was NEVER coming back"), which should be read as a normal word. We
// lower-case all-caps words that look like real words (contain a vowel), while
// leaving consonant-only acronyms (FBI, NBC) and single letters (I, A) alone so
// they are still spelled when that is actually intended.
func normalizeForSpeech(text string) string {
	text = rewriteRomanListMarkers(text)

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

// rewriteRomanListMarkers makes legal-list markers predictable for TTS:
// "(i)", "(ii)", "(iv)" become "one", "two", "four". Only canonical Roman
// numerals through xxxix are accepted so alphabetic clauses such as "(c)"
// remain untouched.
func rewriteRomanListMarkers(text string) string {
	return parenthesizedRomanMarker.ReplaceAllStringFunc(text, func(marker string) string {
		roman := marker[1 : len(marker)-1]
		value, ok := romanListValue(roman)
		if !ok {
			return marker
		}
		return numberWord(value)
	})
}

func romanListValue(roman string) (int, bool) {
	upper := strings.ToUpper(roman)
	values := map[byte]int{'I': 1, 'V': 5, 'X': 10}
	total := 0
	for i := 0; i < len(upper); i++ {
		value, ok := values[upper[i]]
		if !ok {
			return 0, false
		}
		if i+1 < len(upper) && value < values[upper[i+1]] {
			total -= value
		} else {
			total += value
		}
	}
	if total < 1 || total > 39 || canonicalRoman(total) != upper {
		return 0, false
	}
	return total, true
}

func canonicalRoman(value int) string {
	var b strings.Builder
	for _, part := range []struct {
		value int
		text  string
	}{
		{10, "X"},
		{9, "IX"},
		{5, "V"},
		{4, "IV"},
		{1, "I"},
	} {
		for value >= part.value {
			b.WriteString(part.text)
			value -= part.value
		}
	}
	return b.String()
}

func numberWord(value int) string {
	underTwenty := []string{
		"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
		"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen",
		"seventeen", "eighteen", "nineteen",
	}
	if value < len(underTwenty) {
		return underTwenty[value]
	}
	tens := []string{"", "", "twenty", "thirty"}
	word := tens[value/10]
	if remainder := value % 10; remainder != 0 {
		word += "-" + underTwenty[remainder]
	}
	return word
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
