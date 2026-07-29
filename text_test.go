package main

import "testing"

func TestNormalizeForSpeechMakesFormattedTechnicalTextNatural(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "performance introduction",
			input: "How performance should be monitored & tracked per employee — grounded in what `REPOS/workforce/app/Performance` already does.",
			want:  "How performance should be monitored and tracked per employee — grounded in what repos workforce app Performance already does.",
		},
		{
			name:  "markdown stage sequence",
			input: "**2. Three staged assessments, each independently owned.** `self_assessment` → `peer_review` → `manager_review` → `completed`.",
			want:  "2. Three staged assessments, each independently owned. self assessment, peer review, manager review, completed.",
		},
		{
			name:  "methods fields and operators",
			input: "`PerformanceAuthorizationService::employeeCanAssessAs` requires `overall_rating != computed_rating`.",
			want:  "Performance Authorization Service employee Can Assess As requires overall rating does not equal computed rating.",
		},
		{
			name:  "apostrophes mentions and acronyms",
			input: "@Samuel checks the manager`s `PIP` & I`d notify HR.",
			want:  "Samuel checks the manager's PIP and I'd notify HR.",
		},
		{
			name:  "numeric range and ratio",
			input: "Use a 1–5 scale and link the PIP 1:1 to a review.",
			want:  "Use a 1 to 5 scale and link the PIP 1 to 1 to a review.",
		},
		{
			name:  "markdown structure and link",
			input: "# Controls\n\n- Read the [release policy](https://example.com/policy).\n> Keep **outcomes** private.",
			want:  "Controls\n\nRead the release policy.\nKeep outcomes private.",
		},
		{
			name:  "fenced code remains meaningful",
			input: "Verify:\n```php\nPerformanceReviewPresenter::canViewOutcome\nprofile.manager_id\n```",
			want:  "Verify:\nPerformance Review Presenter can View Outcome\nprofile manager id",
		},
		{
			name:  "short acronyms stay acronyms",
			input: "`PIP + HR` and `Auditable + Userstamps`",
			want:  "PIP plus HR and Auditable plus Userstamps",
		},
		{
			name:  "plain unicode prose",
			input: "Café staff already read “ordinary prose”…",
			want:  "Café staff already read \"ordinary prose\"...",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeForSpeech(test.input); got != test.want {
				t.Fatalf("normalizeForSpeech() = %q, want %q", got, test.want)
			}
		})
	}
}

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
