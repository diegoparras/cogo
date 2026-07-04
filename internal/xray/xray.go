// Package xray is the Motor de Veracidad, Phase 1: a DETERMINISTIC "gap meter".
// It takes an AI answer and, per claim, exposes the gap between how strongly the
// language commits and how much grounding it declares — without a model and
// without running anything. It never asserts "true": it flags claims asserted
// hard with no declared basis, opinions dressed as facts, and un-sourced factual
// claims. Green (verified) needs an executed test — that is Phase 2. So here the
// best a claim reaches is yellow; the value is catching the reds.
//
// The twin of internal/suasion (autonomy: "does this push me?"); this is
// veracity: "is this smoke?". See docs/motor-veracidad.md.
package xray

import (
	"regexp"
	"sort"
	"strings"
)

// Claim is one atomic statement with its deterministic radiography.
type Claim struct {
	Text        string `json:"text"`
	Commitment  string `json:"commitment"`  // hedged | neutral | boosted
	Evidence    string `json:"evidence"`    // observed | reported | none
	Falsifiable bool   `json:"falsifiable"` // is it a checkable factual claim (vs opinion)?
	Color       string `json:"color"`       // red | yellow (no green without an executed test)
	Reason      string `json:"reason"`
}

// Report is the whole answer's radiography.
type Report struct {
	Claims  []Claim `json:"claims"`
	Overall string  `json:"overall"`
	Summary string  `json:"summary"`
	Reds    int     `json:"reds"`
	Yellows int     `json:"yellows"`
}

var (
	hedges = []string{"probablemente", "quizás", "quizá", "tal vez", "capaz", "creo que", "me parece", "parece que", "podría", "puede que", "en general", "suele", "a veces", "aparentemente", "diría", "posiblemente", "estimo", "aproximadamente", "más o menos", "no estoy seguro", "i think", "probably", "maybe", "might", "seems"}
	boosters = []string{"seguro", "definitivamente", "siempre", "nunca", "obviamente", "claramente", "sin duda", "por supuesto", "garantizado", "100%", "totalmente", "indudablemente", "es un hecho", "sin lugar a dudas", "certeza", "definitely", "always", "never", "obviously", "guaranteed", "certainly"}
	observed = []string{"corrí", "ejecuté", "medí", "el log", "los logs", "el output", "la salida", "verifiqué", "probé", "el test", "el comando", "confirmé", "reproduje", "ran ", "i ran", "the log", "the output", "i tested", "i verified"}
	reported = []string{"según", "fuente", "documenta", "documentación", "el paper", "el estudio", "reporta", "http://", "https://", "cita", "referencia", "la doc", "according to", "the docs", "source:"}
	opinion  = []string{"mejor", "peor", "debería", "deberías", "hermoso", "elegante", "feo", "importante", "prefiero", "me gusta", "es genial", "es malo", "conviene", "vale la pena", "lindo", "horrible", "should", "better", "worse", "beautiful", "ugly"}
)

var sentenceSplit = regexp.MustCompile(`(?:[.!?]+\s+|\n+|\s*[•\-\*]\s+|;\s+)`)

// Analyze radiographs an answer: segment into claims, then per claim read the
// commitment, the evidentiality, and whether it is falsifiable, and compute a
// deterministic color.
func Analyze(answer string) Report {
	r := Report{Claims: []Claim{}}
	for _, raw := range sentenceSplit.Split(answer, -1) {
		s := strings.TrimSpace(raw)
		if len([]rune(s)) < 15 || strings.HasPrefix(s, "#") {
			continue // too short to be a claim, or a heading
		}
		c := radiograph(s)
		r.Claims = append(r.Claims, c)
		switch c.Color {
		case "red":
			r.Reds++
		case "yellow":
			r.Yellows++
		}
	}
	r.Overall = "yellow"
	if r.Reds > 0 {
		r.Overall = "red"
	}
	if len(r.Claims) == 0 {
		r.Overall = "ungraded"
		r.Summary = "No encontré afirmaciones para radiografiar."
		return r
	}
	r.Summary = plural(len(r.Claims), "afirmación", "afirmaciones") + " · " +
		plural(r.Reds, "en rojo (fuerte sin fundamento / opinión)", "en rojo") + " · " +
		plural(r.Yellows, "en amarillo (sin verificar)", "en amarillo")
	return r
}

func radiograph(s string) Claim {
	low := strings.ToLower(s)
	c := Claim{Text: s, Commitment: "neutral", Evidence: "none", Falsifiable: true}

	if containsAny(low, boosters) {
		c.Commitment = "boosted"
	} else if containsAny(low, hedges) {
		c.Commitment = "hedged"
	}
	if containsAny(low, observed) {
		c.Evidence = "observed"
	} else if containsAny(low, reported) {
		c.Evidence = "reported"
	}
	if containsAny(low, opinion) {
		c.Falsifiable = false
	}

	// Deterministic lattice (Phase 1 — no execution, so no green).
	switch {
	case !c.Falsifiable:
		c.Color, c.Reason = "red", "no falsable: es una opinión o valoración, no un hecho que un test pueda refutar"
	case c.Evidence == "none" && c.Commitment == "boosted":
		c.Color, c.Reason = "red", "afirmado con fuerza pero sin fundamento declarado (el gap más grande)"
	case c.Evidence == "none":
		c.Color, c.Reason = "yellow", "falsable, pero no declara fuente — no testeada"
	case c.Evidence == "reported":
		c.Color, c.Reason = "yellow", "fundamento reportado/citado — no ejecutado acá"
	default: // observed
		c.Color, c.Reason = "yellow", "dice haberlo observado — verde exigiría correr el test (Fase 2)"
	}
	return c
}

func containsAny(low string, words []string) bool {
	for _, w := range words {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

func plural(n int, one, many string) string {
	// one/many already carry the noun; we just prefix the count.
	label := many
	if n == 1 {
		label = one
	}
	return itoa(n) + " " + label
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// Markers returns the lexicon sizes, for a "what it looks for" hint in the UI.
func Markers() map[string]int {
	m := map[string]int{"hedges": len(hedges), "boosters": len(boosters), "observed": len(observed), "reported": len(reported), "opinion": len(opinion)}
	// keep the map access deterministic for any future serialization
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return m
}
