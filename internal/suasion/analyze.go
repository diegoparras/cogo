package suasion

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/llm"
)

// Turn roles. The transcript is ordered oldest-first.
const (
	RoleUser  = "user"
	RoleModel = "model"
)

// Turn is one message of the conversation being analyzed.
type Turn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// Finding is one technique that fired on the turn, with its evidence span. A
// lexicon finding is a signal (yellow at most); a receipt finding carries
// quotes from the transcript and is mechanical; a trajectory finding measures
// sustained pressure; a model proposal is Tier 1 and never rises above yellow.
type Finding struct {
	TechniqueID string     `json:"technique"`
	Name        string     `json:"name"`
	Axes        []string   `json:"axes"`
	Severity    string     `json:"severity"` // the technique's base severity
	Detector    string     `json:"detector"` // "lexicon" | "receipt" | "trajectory" | "model_proposal"
	Evidence    string     `json:"evidence"` // quoted span from the turn
	Receipts    []Receipt  `json:"receipts,omitempty"`
	RedLine     string     `json:"red_line,omitempty"` // set when escalated by the mandate
	Color       core.Color `json:"-"`
	Reason      string     `json:"reason"`
}

// Report is the radiography of one model turn.
type Report struct {
	Mode         string                `json:"mode"` // "mandato" | "informativo"
	Findings     []Finding             `json:"findings"`
	Axes         map[string]core.Color `json:"-"`
	RedLines     []RedLineHit          `json:"red_lines,omitempty"`
	Trajectory   Trajectory            `json:"trajectory"`
	Steelman     *Steelman             `json:"steelman,omitempty"`
	SteelmanNote string                `json:"steelman_note,omitempty"` // why a requested steelman is missing
	Overall      core.Color            `json:"-"`
	Reason       string                `json:"reason"`
}

// Opts are the optional model tiers. Zero value = fully deterministic.
type Opts struct {
	Tier1    llm.Provider // structural proposals; nil or unavailable = off
	Tier2    llm.Provider // adversarial steelman; ideally a DIFFERENT provider than Tier1
	Steelman bool         // request the second opinion (one extra model call)
}

// Analyze reads one model turn. transcript is the conversation so far
// (oldest-first, NOT including the turn under analysis); mandate may be nil.
// Everything here is deterministic: same input, same radiography.
func (e *Engine) Analyze(turn string, transcript []Turn, mandate *Mandate) Report {
	return e.AnalyzeWith(context.Background(), turn, transcript, mandate, Opts{})
}

// AnalyzeWith adds the optional tiers. Tier 1 proposes structural techniques
// the lexicon cannot see — quotes verified literal, capped at yellow. Tier 2
// steelmans the opposite side on request — it informs, it NEVER moves the
// verdict. With both providers off this equals Analyze.
func (e *Engine) AnalyzeWith(ctx context.Context, turn string, transcript []Turn, mandate *Mandate, opts Opts) Report {
	r := Report{Mode: "informativo", Axes: map[string]core.Color{}}
	if mandate.Declared() {
		r.Mode = "mandato"
		r.RedLines = mandate.redLineHits(turn)
	}

	lex := e.lexiconFindings(turn)
	r.Findings = append(r.Findings, lex...)
	r.Findings = append(r.Findings, findReceipts(e, turn, transcript)...)
	// If the hardcoded denial markers found nothing, let the model look for a
	// paraphrased contradiction against the transcript (still verified literal).
	hasReceipt := false
	for _, f := range r.Findings {
		if len(f.Receipts) > 0 {
			hasReceipt = true
		}
	}
	if !hasReceipt && opts.Tier1 != nil && !ownsError(turn) {
		r.Findings = append(r.Findings, modelReceipts(ctx, opts.Tier1, e, turn, transcript)...)
	}
	r.Trajectory = e.computeTrajectory(turn, transcript, mandate, len(lex))
	if g := e.gradualismFinding(r.Trajectory); g != nil {
		r.Findings = append(r.Findings, *g)
	}
	already := map[string]bool{}
	for _, f := range r.Findings {
		already[f.TechniqueID] = true
	}
	for _, f := range e.propose(ctx, opts.Tier1, turn) {
		if !already[f.TechniqueID] {
			r.Findings = append(r.Findings, f)
		}
	}

	// The second opinion is additive and verdict-neutral; when it was asked
	// for and cannot be delivered, the report says so — no silent absence.
	if opts.Steelman {
		switch s, err := e.steelman(ctx, opts.Tier2, turn, mandate); {
		case err != nil:
			r.SteelmanNote = "steelman solicitado pero no disponible: " + err.Error()
		case s == nil:
			r.SteelmanNote = "steelman: el turno no empuja ninguna posición que contrapesar"
		default:
			r.Steelman = s
		}
	}

	// Verdict per finding. The iron rule: a lexicon hit or a model proposal is
	// a signal (yellow); red needs mechanics — receipts, sustained trajectory
	// into a red line, or a strong signal on a declared red line.
	for i := range r.Findings {
		f := &r.Findings[i]
		switch {
		case len(f.Receipts) > 0:
			f.Color = core.Red
			f.Reason = "negación con recibos: la transcripción tiene turnos que la contradicen"
		case f.Detector == "receipt":
			f.Color = core.Yellow
			f.Reason = "marcador de negación sin turnos previos que lo contradigan — verificá el historial"
		case f.Detector == "trajectory" && r.Mode == "mandato" && len(r.RedLines) > 0:
			f.Color = core.Red
			f.RedLine = r.RedLines[0].Line
			f.Reason = "presión sostenida que desemboca en tu línea roja declarada"
		case f.Detector == "trajectory":
			f.Color = core.Yellow
			f.Reason = "presión sostenida a lo largo de la conversación — mirá la tendencia"
		case f.Detector == "model_proposal":
			f.Color = core.Yellow
			f.Reason = "propuesta del modelo local (Tier 1), cita verificada — juzgá vos la estructura"
		case r.Mode == "mandato" && len(r.RedLines) > 0 && (f.Severity == "high" || f.Severity == "critical") && !reinforcesRedLine(turn):
			// A strong signal on a declared red line is a loud yellow, not red:
			// whether the turn PUSHES across the line or merely discusses it is a
			// judgment, not a mechanical fact. Confident red needs receipts.
			f.Color = core.Yellow
			f.RedLine = r.RedLines[0].Line
			f.Reason = "señal fuerte sobre un turno que toca tu línea roja — miralo de cerca"
		default:
			f.Color = core.Yellow
			f.Reason = "señal léxica: técnica presente, no prueba de manipulación"
		}
		for _, ax := range f.Axes {
			if f.Color > r.Axes[ax] {
				r.Axes[ax] = f.Color
			}
		}
	}

	switch {
	case len(r.Findings) == 0:
		r.Overall = core.Green
		r.Reason = "sin señales léxicas ni recibos sobre este turno"
	default:
		r.Overall = core.Yellow
		r.Reason = fmt.Sprintf("%d señal(es) — persuasión presente, revisá las preguntas críticas", len(r.Findings))
		for _, f := range r.Findings {
			if f.Color == core.Red {
				r.Overall = core.Red
				r.Reason = f.Reason
				break
			}
		}
	}
	return r
}

// lexiconFindings matches every compiled marker against the turn. One finding
// per technique, keeping the first evidence span and counting extra hits.
func (e *Engine) lexiconFindings(turn string) []Finding {
	norm := normalize(turn)
	type hit struct {
		evidence string
		extra    int
	}
	hits := map[string]*hit{}
	var order []string
	for _, m := range e.markers {
		byteOff := strings.Index(norm, m.phrase)
		if byteOff < 0 {
			continue
		}
		if h, ok := hits[m.technique]; ok {
			h.extra++
			continue
		}
		from := len([]rune(norm[:byteOff]))
		to := from + len([]rune(m.phrase))
		hits[m.technique] = &hit{evidence: snippet(turn, from, to)}
		order = append(order, m.technique)
	}
	sort.Strings(order) // deterministic output order
	var out []Finding
	for _, id := range order {
		t := e.Ontology.Get(id)
		ev := hits[id].evidence
		if n := hits[id].extra; n > 0 {
			ev = fmt.Sprintf("%s (+%d marcador(es) más)", ev, n)
		}
		out = append(out, Finding{
			TechniqueID: t.ID, Name: t.Name, Axes: t.Axes,
			Severity: t.Severity, Detector: "lexicon", Evidence: ev,
		})
	}
	return out
}
