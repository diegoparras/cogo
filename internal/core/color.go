package core

import (
	"fmt"
	"strings"
)

// Tier is evidence strength. The strongest evidence item sets the note's tier,
// which caps its color: observed can reach green; reported/reasoned cap at
// yellow; none caps at red.
type Tier int

const (
	TierNone     Tier = iota // hypothesis, absence, no ref  -> caps red
	TierReasoned             // inference                     -> caps yellow
	TierReported             // doc, testimony                -> caps yellow
	TierObserved             // direct_log, command_output…   -> caps green
)

var kindTier = map[string]Tier{
	"direct_log":     TierObserved,
	"command_output": TierObserved,
	"test_result":    TierObserved,
	"file_read":      TierObserved,
	"doc":            TierReported,
	"testimony":      TierReported,
	"inference":      TierReasoned,
	"hypothesis":     TierNone,
	"absence":        TierNone,
}

// evidenceTier returns the strongest tier among items that carry a ref. An item
// with no ref does not count — no ref, treat as none.
func evidenceTier(ev []Evidence) Tier {
	best := TierNone
	for _, e := range ev {
		if strings.TrimSpace(e.Ref) == "" || e.Status == EvBroken {
			continue // no ref, or a ref that provably does not resolve, carries no weight
		}
		if t, ok := kindTier[e.Kind]; ok && t > best {
			best = t
		}
	}
	return best
}

// hasBrokenEvidence reports whether any evidence item was checked and did not
// resolve — used to explain a red note as "broken ref" rather than "no evidence".
func hasBrokenEvidence(ev []Evidence) bool {
	for _, e := range ev {
		if e.Status == EvBroken {
			return true
		}
	}
	return false
}

// hasDriftedEvidence reports whether any cited file changed since the note was
// verified — the evidence moved under it, so it can no longer be green.
func hasDriftedEvidence(ev []Evidence) bool {
	for _, e := range ev {
		if e.Status == EvDrifted {
			return true
		}
	}
	return false
}

// windowDays returns the freshness window per type (in days). One window per
// type derives both thresholds: stale_at = last_verified + window (-> yellow),
// expiry = last_verified + 2×window (-> red). Mistakes never decay and are
// handled before this is called.
func windowDays(noteType string) int {
	if ventanas != nil {
		if d, ok := ventanas(noteType); ok {
			return d
		}
	}
	switch noteType {
	case "constraint":
		return 365
	case "decision", "architecture":
		return 180
	case "runbook":
		return 90
	case "bug":
		return 60
	case "command":
		return 30
	default:
		return 90 // conservative default for an unknown type
	}
}

// ventanas, si está instalada, decide la ventana de frescura de cada tipo. La
// tabla de arriba queda como el default que ve un COGO sin configurar, y sigue
// siendo la respuesta cuando el hook dice que no sabe.
//
// Es el mismo patrón de SetMotor y SetWriteHook: core no lee archivos ni conoce
// la configuración, y quien arranca el programa decide qué le inyecta.
var ventanas func(noteType string) (int, bool)

// SetVentanas instala la tabla de frescura. nil vuelve a los valores de core.
func SetVentanas(f func(noteType string) (int, bool)) { ventanas = f }

// Verdict is the computed color plus the clause that decided it. The reason is
// what makes any color auditable.
type Verdict struct {
	Color   Color
	Reason  string
	StaleAt Date
}

// Evaluate computes the color of one note within its vault. vault must contain
// the note itself (keyed by ID) plus every note it depends on. contradictions
// is the set of note IDs touched by an open hard contradiction (from lint);
// nil is fine. today is injected so the result is deterministic and testable.
// motorExterno, si está instalado, reemplaza el cálculo del color. Es el punto
// ÚNICO del corte: en vez de cambiar los diez llamadores repartidos entre el
// visor, la CLI y el servidor MCP, se cambia el motor de acá para adentro.
//
// Sigue el mismo patrón que SetWriteHook: core se mantiene puro y sin
// dependencias, y quien arranca el programa decide qué le inyecta. Y como es una
// variable, volver al motor anterior es no instalarlo — la marcha atrás no
// necesita un release.
var motorExterno func(vault map[string]*Note, contradictions map[string]bool, today Date) map[string]Verdict

// SetMotor instala un motor de color alternativo. Pasar nil vuelve al de core.
func SetMotor(f func(vault map[string]*Note, contradictions map[string]bool, today Date) map[string]Verdict) {
	motorExterno = f
}

func Evaluate(n *Note, vault map[string]*Note, contradictions map[string]bool, today Date) Verdict {
	if motorExterno != nil {
		// Una nota se evalúa en el contexto de su vault: su color depende de
		// aquello de lo que depende, así que no hay atajo por nota suelta.
		if v, ok := motorExterno(vault, contradictions, today)[n.ID]; ok {
			return v
		}
	}
	return newEvaluator(vault, contradictions, today).evaluate(n.ID)
}

// EvaluateVault computes every note's color in one memoized pass.
func EvaluateVault(vault map[string]*Note, contradictions map[string]bool, today Date) map[string]Verdict {
	if motorExterno != nil {
		return motorExterno(vault, contradictions, today)
	}
	return EvaluateVaultCore(vault, contradictions, today)
}

// EvaluateVaultCore calcula con las reglas de core, sin pasar por el motor
// instalado. Existe para que un motor externo pueda apoyarse en él —por ejemplo
// para las ventanas de frescura por tipo, que son una tabla de core— sin
// llamarse a sí mismo para siempre.
func EvaluateVaultCore(vault map[string]*Note, contradictions map[string]bool, today Date) map[string]Verdict {
	e := newEvaluator(vault, contradictions, today)
	out := make(map[string]Verdict, len(vault))
	for id := range vault {
		out[id] = e.evaluate(id)
	}
	return out
}

func newEvaluator(vault map[string]*Note, contradictions map[string]bool, today Date) *evaluator {
	return &evaluator{
		vault:          vault,
		contradictions: contradictions,
		today:          today,
		memo:           map[string]Verdict{},
		inProgress:     map[string]bool{},
	}
}

type evaluator struct {
	vault          map[string]*Note
	contradictions map[string]bool
	today          Date
	memo           map[string]Verdict
	inProgress     map[string]bool
}

func (e *evaluator) evaluate(id string) Verdict {
	if v, ok := e.memo[id]; ok {
		return v
	}
	n, ok := e.vault[id]
	if !ok {
		// A depends_on points at a note that isn't here: nothing rests safely
		// on something we can't see.
		return Verdict{Red, fmt.Sprintf("falta la nota de la que depende: %q", id), Date{}}
	}
	if e.inProgress[id] {
		// A cycle in depends_on: nothing in it can be trusted above red.
		return Verdict{Red, "ciclo de dependencias", Date{}}
	}
	e.inProgress[id] = true
	v := e.compute(n)
	delete(e.inProgress, id)
	e.memo[id] = v
	return v
}

// compute applies §4 top-down: the first clause that forces a color wins, and
// the reason records which clause decided. A note is green only when nothing
// pulls it down.
func (e *evaluator) compute(n *Note) Verdict {
	if n.Type == "mistake" {
		return Verdict{Ungraded, "error registrado: es informativo, no se gradúa por confianza", Date{}}
	}
	// Una brecha no se gradúa porque no afirma nada: es una pregunta abierta.
	// Pintarla de roja —tentador, porque no tiene evidencia— la convertiría en
	// una mala afirmación en vez de una buena pregunta.
	if EsBrecha(n) {
		if k := len(n.Blocks); k > 0 {
			return Verdict{Ungraded, fmt.Sprintf("pregunta abierta: no se sabe todavía, y hay %d decisión(es) esperándola", k), Date{}}
		}
		return Verdict{Ungraded, "pregunta abierta: es algo que el proyecto no sabe, no una afirmación", Date{}}
	}

	w := windowDays(n.Type)
	staleAt := n.LastVerified.AddDays(w)
	expiry := n.LastVerified.AddDays(2 * w)
	tier := evidenceTier(n.Evidence)

	var depRed, depYellow string
	for _, d := range n.DependsOn {
		switch e.evaluate(d).Color {
		case Red:
			if depRed == "" {
				depRed = d
			}
		case Yellow:
			if depYellow == "" {
				depYellow = d
			}
		}
	}

	// RED — evaluate top-down; first match that forces red wins.
	switch {
	case e.contradictions[n.ID]:
		return Verdict{Red, "una contradicción abierta toca esta nota", staleAt}
	case depRed != "":
		return Verdict{Red, fmt.Sprintf("depende de la nota roja %q", depRed), staleAt}
	case tier == TierNone:
		if hasBrokenEvidence(n.Evidence) {
			return Verdict{Red, "la evidencia citada no resuelve (referencia rota)", staleAt}
		}
		return Verdict{Red, "sin evidencia observada ni reportada", staleAt}
	case e.today.After(expiry):
		return Verdict{Red, "expirada: pasó el doble de la ventana de frescura sin re-verificar", staleAt}
	}

	// YELLOW — not red, but something keeps it below green.
	switch {
	case hasDriftedEvidence(n.Evidence):
		return Verdict{Yellow, fmt.Sprintf("cambió lo que la nota cita (%s) — re-verificá",
			strings.Join(DriftedRefs(n), ", ")), staleAt}
	case tier == TierReported || tier == TierReasoned:
		return Verdict{Yellow, "la evidencia es reportada o razonada (tope: amarillo)", staleAt}
	case n.Check.Status != "passed": // tier is observed here
		return Verdict{Yellow, "evidencia observada, pero el check no pasó", staleAt}
	case e.today.After(staleAt):
		return Verdict{Yellow, "vencida: pasó su fecha de frescura (todavía no expiró)", staleAt}
	case depYellow != "":
		return Verdict{Yellow, fmt.Sprintf("depende de la nota amarilla %q", depYellow), staleAt}
	}

	// GREEN — observed, check passed, fresh, every dependency green, no contradiction.
	return Verdict{Green, "evidencia observada, check pasado, fresca, dependencias verdes, sin contradicción", staleAt}
}

// TierDeEvidencia expone la fuerza de la evidencia de una nota. Es el techo que
// la evidencia le impone al color: sin evidencia observada no hay verificación
// que alcance para llegar a verde.
func TierDeEvidencia(ev []Evidence) Tier { return evidenceTier(ev) }
