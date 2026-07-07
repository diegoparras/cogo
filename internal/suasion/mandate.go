package suasion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Mandate is the user's declared reference: what they are NOT willing to do or
// believe. Without it, manipulation and legitimate persuasion are
// indistinguishable — the engine then degrades to informative mode (it names
// tactics but does not grade autonomy).
type Mandate struct {
	Goal     string   `json:"goal,omitempty" yaml:"goal,omitempty"`
	RedLines []string `json:"red_lines,omitempty" yaml:"red_lines,omitempty"`
}

// MandatePath is the canonical location of the persisted mandate inside a
// vault — next to llm.json, private runtime state, never packed as a note.
func MandatePath(vaultDir string) string {
	return filepath.Join(vaultDir, ".cogo", "mandate.json")
}

// LoadMandate reads a persisted mandate; nil when missing or unreadable — the
// callers treat that as "not declared", never as an error.
func LoadMandate(path string) *Mandate {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m Mandate
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	if !m.Declared() && m.Goal == "" {
		return nil
	}
	return &m
}

// SaveMandate persists the mandate for future calls (web and MCP share it).
func SaveMandate(path string, m *Mandate) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

// Declared reports whether there is anything to measure drift against.
func (m *Mandate) Declared() bool {
	return m != nil && len(m.RedLines) > 0
}

// RedLineHit is a declared red line whose content words appear in the turn.
type RedLineHit struct {
	Line    string   `json:"line"`
	Matched []string `json:"matched"` // the content words found in the turn
}

// redLineHits reports which red lines the turn touches. A line is hit when the
// turn shares at least two of its content words, or at least half of them.
// Words match on equality OR a shared 4+ char prefix, so lemma families meet
// across plurals, conjugation tails and enclitics ("consultes" ~ "consultarlo")
// without the risk of over-collapsing stem-changing diphthongs. Deterministic
// and auditable — the matched words are the evidence.
func (m *Mandate) redLineHits(turn string) []RedLineHit {
	if !m.Declared() {
		return nil
	}
	turnWords := contentWords(turn, 3)
	var hits []RedLineHit
	for _, line := range m.RedLines {
		words := contentWords(line, 3)
		if len(words) == 0 {
			continue
		}
		var matched []string
		for _, w := range words {
			if overlapsAny(w, turnWords) {
				matched = append(matched, w)
			}
		}
		if len(matched) >= 2 || len(matched)*2 >= len(words) {
			hits = append(hits, RedLineHit{Line: line, Matched: matched})
		}
	}
	return hits
}

// refusalCues mark a turn that AGREES with not crossing the red line — it
// reinforces the boundary rather than pushing across it. Escalation to red is
// about pressure toward the line; a turn that says "no vas a hacerlo, y está
// bien" must not be flagged just because it shares words with the line.
var refusalCues = []string{
	"no vas a", "no hace falta", "sin necesidad", "mejor no ", "no te conviene",
	"no es necesario", "no tenes que", "respetamos que no", "esta bien no",
	"no deberias", "no es sano", "lo mas sano",
}

// reinforcesRedLine reports whether the turn reads as supporting the boundary
// (a refusal cue present), so mandate escalation should not fire red on it.
func reinforcesRedLine(turn string) bool {
	n := normalize(turn)
	for _, c := range refusalCues {
		if strings.Contains(n, c) {
			return true
		}
	}
	return false
}

// overlapsAny reports whether w equals, or shares a 4+ char prefix with, any
// word in set. Both sides are already normalized+stemmed ASCII.
func overlapsAny(w string, set []string) bool {
	for _, t := range set {
		if w == t {
			return true
		}
		if len(w) >= 4 && len(t) >= 4 && (strings.HasPrefix(w, t) || strings.HasPrefix(t, w)) {
			return true
		}
	}
	return false
}
