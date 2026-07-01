package suasion

import "strings"

// Normalization maps the text rune-by-rune (same rune count in and out) so a
// match position in the normalized text is the same rune position in the
// original — evidence snippets quote the original verbatim.
var normRune = map[rune]rune{
	'Á': 'a', 'É': 'e', 'Í': 'i', 'Ó': 'o', 'Ú': 'u', 'Ü': 'u', 'Ñ': 'n',
	'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ú': 'u', 'ü': 'u', 'ñ': 'n',
	'’': '\'', '‘': '\'', '“': '"', '”': '"',
}

func normalize(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		if m, ok := normRune[r]; ok {
			rs[i] = m
		} else if r >= 'A' && r <= 'Z' {
			rs[i] = r + ('a' - 'A')
		}
	}
	return string(rs)
}

// Spanish stopwords for content-word overlap. Small on purpose: receipts only
// need to rank candidate turns, not parse them.
var stopwords = map[string]bool{
	"de": true, "la": true, "que": true, "el": true, "en": true, "y": true,
	"a": true, "los": true, "se": true, "del": true, "las": true, "un": true,
	"por": true, "con": true, "no": true, "una": true, "su": true, "para": true,
	"es": true, "al": true, "lo": true, "como": true, "mas": true, "pero": true,
	"sus": true, "le": true, "ya": true, "o": true, "fue": true, "este": true,
	"ha": true, "si": true, "porque": true, "esta": true, "son": true,
	"entre": true, "cuando": true, "muy": true, "sin": true, "sobre": true,
	"ser": true, "tiene": true, "tambien": true, "me": true, "hasta": true,
	"hay": true, "donde": true, "han": true, "quien": true, "estan": true,
	"desde": true, "todo": true, "nos": true, "durante": true, "todos": true,
	"uno": true, "les": true, "ni": true, "contra": true, "otros": true,
	"ese": true, "eso": true, "ante": true, "ellos": true, "e": true,
	"esto": true, "mi": true, "antes": true, "algunos": true, "unos": true,
	"yo": true, "otro": true, "otras": true, "otra": true, "tanto": true,
	"esa": true, "estos": true, "mucho": true, "quienes": true, "nada": true,
	"muchos": true, "cual": true, "poco": true, "ella": true, "estar": true,
	"estas": true, "algunas": true, "algo": true, "nosotros": true,
	"vos": true, "te": true, "tu": true, "dije": true, "dijo": true,
	"nunca": true, "hoy": true, "voy": true, "vas": true, "eres": true,
	"sos": true, "soy": true,
}

// contentWords returns the normalized words of s that carry meaning: not a
// stopword and at least minLen runes long.
func contentWords(s string, minLen int) []string {
	var out []string
	for _, w := range strings.FieldsFunc(normalize(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len([]rune(w)) >= minLen && !stopwords[w] {
			out = append(out, w)
		}
	}
	return out
}

// sentenceAround returns the sentence of s (rune offsets) containing the rune
// range [from, to). Boundaries are ., !, ?, ; and newline.
func sentenceAround(s string, from, to int) string {
	rs := []rune(s)
	isBoundary := func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == ';' || r == '\n'
	}
	start := from
	for start > 0 && !isBoundary(rs[start-1]) {
		start--
	}
	end := to
	for end < len(rs) && !isBoundary(rs[end]) {
		end++
	}
	return strings.TrimSpace(string(rs[start:end]))
}

// snippet quotes the original text around a rune range, elided to a readable
// window.
func snippet(s string, from, to int) string {
	const margin = 40
	rs := []rune(s)
	start, end := from-margin, to+margin
	pre, post := "…", "…"
	if start <= 0 {
		start, pre = 0, ""
	}
	if end >= len(rs) {
		end, post = len(rs), ""
	}
	return pre + strings.TrimSpace(string(rs[start:end])) + post
}
