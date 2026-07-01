package suasion

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/llm"
)

// Tier 1: a small local model classifies STRUCTURE (false binaries, loaded
// presuppositions) that no lexicon can catch. Bounded on both ends — the
// shortlist is closed and every quote must be literal text of the turn or the
// proposal is dropped. The model proposes; it never colors above yellow.

// proposalShortlist: structural techniques with no usable lexical marker,
// single-turn detectable, high or critical severity. Closed and deterministic.
func (e *Engine) proposalShortlist() []*Technique {
	covered := map[string]bool{}
	for _, m := range e.markers {
		covered[m.technique] = true
	}
	var out []*Technique
	for _, d := range e.Ontology.Disciplines {
		for i := range d.Techniques {
			t := &d.Techniques[i]
			if covered[t.ID] || (t.Severity != "high" && t.Severity != "critical") ||
				(t.Trajectory != "single" && t.Trajectory != "both") {
				continue
			}
			structural := false
			for _, det := range t.Detectors {
				if det.Type == "structure" || det.Type == "pragmatic" || det.Type == "speech_act" {
					structural = true
				}
			}
			if structural {
				out = append(out, t)
			}
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Severity != out[b].Severity {
			return out[a].Severity == "critical"
		}
		return out[a].ID < out[b].ID
	})
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

// Propose asks the provider one bounded question: which shortlist techniques
// structurally fire on this turn. Every returned quote is verified to be a
// literal fragment of the turn; anything else is dropped — the teeth stay
// deterministic even when a model participates.
func (e *Engine) propose(ctx context.Context, p llm.Provider, turn string) []Finding {
	if p == nil || !p.Available() {
		return nil
	}
	shortlist := e.proposalShortlist()
	if len(shortlist) == 0 {
		return nil
	}
	byID := map[string]*Technique{}
	var b strings.Builder
	b.WriteString("Sos un analizador ACOTADO de estructura retórica. No opinás sobre verdad ni sobre manipulación; solo reconocés formas.\n\n")
	b.WriteString("Turno a analizar:\n---\n" + turn + "\n---\n\nCatálogo cerrado:\n")
	for _, t := range shortlist {
		byID[t.ID] = t
		fmt.Fprintf(&b, "- %s — %s\n", t.ID, strings.TrimSpace(t.Definition))
	}
	b.WriteString("\nRespondé SOLO líneas con el formato exacto:\nid | cita literal del turno que muestra esa estructura\n")
	b.WriteString("Máximo 3 líneas, solo ids del catálogo, la cita debe ser un fragmento LITERAL del turno. Si ninguna aplica respondé exactamente: NINGUNA\n")

	out, err := p.Complete(ctx, b.String())
	if err != nil {
		return nil
	}
	normTurn := normalize(turn)
	var findings []Finding
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.Trim(strings.TrimSpace(parts[0]), "-• ")
		quote := strings.Trim(strings.TrimSpace(parts[1]), `"“”'`)
		t, ok := byID[id]
		if !ok || seen[id] || quote == "" {
			continue // outside the closed shortlist, or duplicate
		}
		byteOff := strings.Index(normTurn, normalize(quote))
		if byteOff < 0 {
			continue // fabricated quote: dropped, no exceptions
		}
		seen[id] = true
		from := len([]rune(normTurn[:byteOff]))
		findings = append(findings, Finding{
			TechniqueID: t.ID, Name: t.Name, Axes: t.Axes, Severity: t.Severity,
			Detector: "model_proposal",
			Evidence: snippet(turn, from, from+len([]rune(quote))),
		})
		if len(findings) == 3 {
			break
		}
	}
	return findings
}
