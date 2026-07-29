package main

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	parenthesizedRomanMarker = regexp.MustCompile(`\(([iIvVxX]+)\)`)
	intraWordBacktick        = regexp.MustCompile("([\\p{L}\\p{N}])`([\\p{L}])")
	markdownImage            = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	markdownLink             = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	inlineCode               = regexp.MustCompile("`([^`\\n]+)`")
	markdownLinePrefix       = regexp.MustCompile(`^[ \t]{0,3}(#{1,6}[ \t]+|>[ \t]?|[-+*][ \t]+)`)
	numericRange             = regexp.MustCompile(`(\d)[ \t]*[–—][ \t]*(\d)`)
	numericRatio             = regexp.MustCompile(`(\d)[ \t]*:[ \t]*(\d)`)
	atMention                = regexp.MustCompile(`@([\p{L}\p{N}_-]+)`)
	spaceBeforePunctuation   = regexp.MustCompile(`[ \t]+([,.;:!?])`)
)

// normalizeForSpeech rewrites text so a TTS engine pronounces it naturally.
//
// The stored text remains unchanged: this function only prepares the copy sent
// to Kokoro. Besides fixing ALL-CAPS emphasis and Roman list markers, it removes
// visual Markdown syntax and turns code-like identifiers into spoken words.
func normalizeForSpeech(text string) string {
	text = normalizeSpeechUnicode(text)
	text = intraWordBacktick.ReplaceAllString(text, "$1'$2")
	text = simplifyMarkdownForSpeech(text)
	text = rewriteSpeechSymbols(text)
	text = rewriteRomanListMarkers(text)
	text = collapseSpeechWhitespace(text)

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

func normalizeSpeechUnicode(text string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		"\u00a0", " ",
		"\u200b", "",
		"\ufeff", "",
		"\u2018", "'",
		"\u2019", "'",
		"\u201b", "'",
		"\u201c", "\"",
		"\u201d", "\"",
		"\u2026", "...",
	)
	return replacer.Replace(text)
}

// simplifyMarkdownForSpeech removes formatting while preserving what a reader
// can see. Fenced and inline code are retained, but their identifiers are made
// pronounceable rather than being omitted.
func simplifyMarkdownForSpeech(text string) string {
	text = markdownImage.ReplaceAllString(text, "$1")
	text = markdownLink.ReplaceAllString(text, "$1")

	lines := strings.Split(text, "\n")
	inFence := false
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			line = naturalizeCodeText(line)
		} else {
			line = markdownLinePrefix.ReplaceAllString(line, "")
			line = inlineCode.ReplaceAllStringFunc(line, func(code string) string {
				return naturalizeCodeText(code[1 : len(code)-1])
			})
		}
		kept = append(kept, line)
	}

	text = strings.Join(kept, "\n")
	text = strings.NewReplacer(
		"**", "",
		"__", "",
		"~~", "",
		"*", "",
		"_", " ",
		"`", "",
	).Replace(text)
	return text
}

func naturalizeCodeText(text string) string {
	text = strings.NewReplacer(
		"!=", " does not equal ",
		">=", " greater than or equal to ",
		"<=", " less than or equal to ",
		"==", " equals ",
		"&&", " and ",
		"||", " or ",
		"::", " ",
		"->", " ",
		"=>", " ",
		"/", " ",
		"\\", " ",
		"_", " ",
		".", " ",
		"+", " plus ",
		"=", " equals ",
	).Replace(text)

	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text) + len(text)/8)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			previous := runes[i-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) ||
				(unicode.IsUpper(previous) && nextIsLower) {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func rewriteSpeechSymbols(text string) string {
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = numericRange.ReplaceAllString(text, "$1 to $2")
	text = numericRatio.ReplaceAllString(text, "$1 to $2")
	text = atMention.ReplaceAllString(text, "$1")
	return strings.NewReplacer(
		"→", ", ",
		"⇒", ", ",
		"➜", ", ",
		"➔", ", ",
		"->", ", ",
		"=>", ", ",
		"&", " and ",
	).Replace(text)
}

func collapseSpeechWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	previousBlank := true
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		line = spaceBeforePunctuation.ReplaceAllString(line, "$1")
		if line == "" {
			if !previousBlank {
				cleaned = append(cleaned, "")
			}
			previousBlank = true
			continue
		}
		cleaned = append(cleaned, line)
		previousBlank = false
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return strings.Join(cleaned, "\n")
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
