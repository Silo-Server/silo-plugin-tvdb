package provider

// iso639_1To2 maps ISO 639-1 two-letter codes to ISO 639-2/B three-letter
// codes used by the TVDB v4 API translation endpoints.
var iso639_1To2 = map[string]string{
	"en": "eng", "ja": "jpn", "fr": "fra", "de": "deu",
	"es": "spa", "it": "ita", "pt": "por", "zh": "zho",
	"ko": "kor", "ru": "rus", "nl": "nld", "pl": "pol",
	"sv": "swe", "da": "dan", "no": "nor", "fi": "fin",
	"tr": "tur", "cs": "ces", "hu": "hun", "he": "heb",
	"ar": "ara", "th": "tha", "id": "ind", "vi": "vie",
	"uk": "ukr", "ro": "ron", "bg": "bul", "hr": "hrv",
	"sk": "slk", "el": "ell", "sl": "slv", "ms": "msa",
	"hi": "hin", "ta": "tam", "te": "tel", "bn": "ben",
	"fa": "fas",
}

// toLang3 converts a language tag (ISO 639-1 or already 3-letter) to the
// ISO 639-2 three-letter code the TVDB API uses in URL paths.
// Input may include region suffixes (e.g. "fr-CA" → "fra").
// Returns "eng" for unknown or empty inputs.
func toLang3(lang string) string {
	norm := normalizeLanguageTag(lang)
	if len(norm) == 3 {
		return norm
	}
	if code, ok := iso639_1To2[norm]; ok {
		return code
	}
	return "eng"
}

// needsTranslation returns true if the requested language differs from the
// record's original language, meaning translations should be fetched.
// Returns false if requestedLang is empty (no preference set).
// Returns true if originalLang is empty (unknown original, fetch to be safe).
// Both inputs are normalized to 3-letter codes for reliable comparison
// (e.g. "ja" and "jpn" correctly resolve to the same language).
func needsTranslation(requestedLang, originalLang string) bool {
	if requestedLang == "" {
		return false
	}
	if originalLang == "" {
		return true
	}
	return toLang3(requestedLang) != toLang3(originalLang)
}

// toLang1 converts a TVDB language tag into the ISO 639-1 two-letter form the
// host selector expects for exact language matching. Unknown three-letter codes
// fall back to the normalized input.
func toLang1(lang string) string {
	norm := normalizeLanguageTag(lang)
	if norm == "" || len(norm) == 2 {
		return norm
	}
	for code2, code3 := range iso639_1To2 {
		if code3 == norm {
			return code2
		}
	}
	return norm
}
