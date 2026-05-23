package provider

import "testing"

func TestToLang3(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "en", want: "eng"},
		{input: "fr-CA", want: "fra"},
		{input: "eng", want: "eng"},
		{input: "", want: "eng"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := toLang3(tt.input); got != tt.want {
				t.Fatalf("toLang3(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToLang1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "eng", want: "en"},
		{input: "fra", want: "fr"},
		{input: "fr-CA", want: "fr"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := toLang1(tt.input); got != tt.want {
				t.Fatalf("toLang1(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNeedsTranslation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		requestedLang string
		originalLang  string
		want          bool
	}{
		{name: "english user english show", requestedLang: "en", originalLang: "eng", want: false},
		{name: "english user japanese show", requestedLang: "en", originalLang: "jpn", want: true},
		{name: "english user unknown original", requestedLang: "en", originalLang: "", want: true},
		{name: "no requested language", requestedLang: "", originalLang: "jpn", want: false},
		{name: "japanese user japanese show", requestedLang: "ja", originalLang: "jpn", want: false},
		{name: "french user english show", requestedLang: "fr", originalLang: "eng", want: true},
		{name: "both empty", requestedLang: "", originalLang: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsTranslation(tt.requestedLang, tt.originalLang); got != tt.want {
				t.Fatalf("needsTranslation(%q, %q) = %v, want %v",
					tt.requestedLang, tt.originalLang, got, tt.want)
			}
		})
	}
}

func TestLanguageMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		candidate string
		want      bool
	}{
		{name: "exact normalized", requested: "eng", candidate: "eng", want: true},
		{name: "region to base", requested: "fr-CA", candidate: "fra", want: true},
		{name: "base to region", requested: "fr", candidate: "fr-CA", want: true},
		{name: "different languages", requested: "en", candidate: "fra", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := languageMatches(tt.requested, tt.candidate); got != tt.want {
				t.Fatalf("languageMatches(%q, %q) = %v, want %v", tt.requested, tt.candidate, got, tt.want)
			}
		})
	}
}
