package suasion

import (
	"sort"
	"strings"
)

// The receipts: COGO is the wire, so it holds the immutable transcript.
// Gaslighting depends on the victim not being able to verify the past; here a
// denial marker triggers a deterministic search of the model's own prior turns
// and the quotes are shown side by side. No NLI model in phase 1 — overlap of
// content words ranks the candidates, and the human judges the quotes.

// denialMarkers gate the receipt search. Curated in code (not extracted from
// the ontology) because they carry a precise role: denying the past.
var denialMarkers = []struct{ phrase, technique string }{
	{"yo nunca dije", "dark_psychology.gaslighting"},
	{"nunca dije eso", "dark_psychology.gaslighting"},
	{"no dije eso", "dark_psychology.gaslighting"},
	{"eso no fue lo que dije", "dark_psychology.gaslighting"},
	{"no fue lo que dije", "dark_psychology.gaslighting"},
	{"nunca afirme eso", "dark_psychology.gaslighting"},
	{"yo nunca afirme", "dark_psychology.gaslighting"},
	{"eso no fue lo que paso", "dark_psychology.gaslighting"},
	{"eso nunca paso", "dark_psychology.gaslighting"},
	{"lo estas imaginando", "dark_psychology.gaslighting"},
	{"te estas confundiendo", "dark_psychology.gaslighting"},
	{"estas malinterpretando", "dark_psychology.gaslighting"},
	{"tu memoria te falla", "dark_psychology.gaslighting"},
	{"yo nunca prometi", "dark_psychology.future_faking"},
	{"nunca prometi eso", "dark_psychology.future_faking"},
	{"yo no prometi", "dark_psychology.future_faking"},
	{"el criterio siempre fue", "dark_psychology.moving_goalposts"},
	{"siempre pedimos eso", "dark_psychology.moving_goalposts"},
	{"eso siempre estuvo incluido", "dark_psychology.moving_goalposts"},
}

// Receipt is one prior model turn that lexically overlaps the denial — the
// quote the human checks against the denial, side by side.
type Receipt struct {
	TurnIndex int      `json:"turn_index"` // index into the transcript
	Quote     string   `json:"quote"`
	Shared    []string `json:"shared"` // content words in common with the denial
}

// findReceipts scans the model turn for denial markers; on a hit it ranks the
// model's prior turns by shared content words with the denial sentence and
// returns the top candidates as receipts.
func findReceipts(e *Engine, turn string, transcript []Turn) []Finding {
	norm := normalize(turn)
	var findings []Finding
	for _, dm := range denialMarkers {
		byteOff := strings.Index(norm, dm.phrase)
		if byteOff < 0 {
			continue
		}
		from := len([]rune(norm[:byteOff]))
		to := from + len([]rune(dm.phrase))
		sentence := sentenceAround(turn, from, to)

		want := map[string]bool{}
		for _, w := range contentWords(sentence, 4) {
			want[w] = true
		}
		minShared := 2
		if len(want) < 2 {
			minShared = 1
		}

		var receipts []Receipt
		for i, t := range transcript {
			if t.Role != RoleModel {
				continue
			}
			var shared []string
			for _, w := range contentWords(t.Text, 4) {
				if want[w] {
					shared = append(shared, w)
					delete(want, w) // count each word once per turn
				}
			}
			for _, w := range shared {
				want[w] = true // restore for the next candidate turn
			}
			if len(shared) >= minShared {
				receipts = append(receipts, Receipt{TurnIndex: i, Quote: clip(t.Text, 240), Shared: shared})
			}
		}
		sort.SliceStable(receipts, func(a, b int) bool {
			return len(receipts[a].Shared) > len(receipts[b].Shared)
		})
		if len(receipts) > 2 {
			receipts = receipts[:2]
		}

		t := e.Ontology.Get(dm.technique)
		findings = append(findings, Finding{
			TechniqueID: t.ID,
			Name:        t.Name,
			Axes:        t.Axes,
			Severity:    t.Severity,
			Detector:    "receipt",
			Evidence:    sentence,
			Receipts:    receipts,
		})
		// One receipt finding per technique is enough for one turn.
		break
	}
	return findings
}

func clip(s string, max int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max]) + "…"
}
