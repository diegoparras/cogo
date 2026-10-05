package suasion

import (
	"regexp"
	"strings"
	"sync"
)

// Engine is the phase-1 deterministic analyzer: lexical markers compiled from
// the ontology plus receipt detection over the transcript. No model anywhere —
// these are the teeth. See docs/motor-autonomia.md.
type Engine struct {
	Ontology *Ontology
	markers  []marker
}

// marker is one usable lexical phrase tied back to its technique.
type marker struct {
	phrase    string // normalized
	technique string
}

var quoted = regexp.MustCompile(`'([^']+)'`)

// Marker quality gate: substring matching over free text needs phrases long
// enough to be unambiguous. Short common phrases ("ya que", "deberías") would
// fire constantly — and the cost of a false positive is the user's trust.
const (
	minMarkerWords = 3
	minMarkerRunes = 12
)

func usablePhrase(p string) bool {
	if strings.ContainsAny(p, "…") || strings.Contains(p, "...") {
		return false // template, not a literal phrase
	}
	return len(strings.Fields(p)) >= minMarkerWords || len([]rune(p)) >= minMarkerRunes
}

// NewEngine compiles the lexicon out of the ontology's lexicon detectors. The
// ontology is the single source of rules; nothing is duplicated in code.
func NewEngine(o *Ontology) *Engine {
	e := &Engine{Ontology: o}
	for _, d := range o.Disciplines {
		for _, t := range d.Techniques {
			for _, det := range t.Detectors {
				if det.Type != "lexicon" {
					continue
				}
				for _, m := range quoted.FindAllStringSubmatch(det.Signal, -1) {
					if p := strings.TrimSpace(m[1]); usablePhrase(p) {
						e.markers = append(e.markers, marker{normalize(p), t.ID})
					}
				}
			}
		}
	}
	return e
}

var (
	defaultOnce   sync.Once
	defaultEngine *Engine
	defaultErr    error
)

// Default returns the engine over the embedded ontology, built once.
func Default() (*Engine, error) {
	defaultOnce.Do(func() {
		o, err := Load()
		if err == nil {
			err = o.Validate()
		}
		if err != nil {
			defaultErr = err
			return
		}
		defaultEngine = NewEngine(o)
	})
	return defaultEngine, defaultErr
}

// Coverage returns how many techniques have at least one usable lexical
// marker — the honest reach of phase 1. The rest need the later tiers.
func (e *Engine) Coverage() (covered, total int) {
	seen := map[string]bool{}
	for _, m := range e.markers {
		seen[m.technique] = true
	}
	return len(seen), e.Ontology.Len()
}
