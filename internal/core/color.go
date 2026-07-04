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

// windowDays returns the freshness window per type (in days). One window per
// type derives both thresholds: stale_at = last_verified + window (-> yellow),
// expiry = last_verified + 2×window (-> red). Mistakes never decay and are
// handled before this is called.
func windowDays(noteType string) int {
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
func Evaluate(n *Note, vault map[string]*Note, contradictions map[string]bool, today Date) Verdict {
	return newEvaluator(vault, contradictions, today).evaluate(n.ID)
}

// EvaluateVault computes every note's color in one memoized pass.
func EvaluateVault(vault map[string]*Note, contradictions map[string]bool, today Date) map[string]Verdict {
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
		return Verdict{Red, fmt.Sprintf("missing dependency note %q", id), Date{}}
	}
	if e.inProgress[id] {
		// A cycle in depends_on: nothing in it can be trusted above red.
		return Verdict{Red, "dependency cycle", Date{}}
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
		return Verdict{Ungraded, "mistake: informational, not graded by confidence", Date{}}
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
		return Verdict{Red, "open hard contradiction touches the note", staleAt}
	case depRed != "":
		return Verdict{Red, fmt.Sprintf("depends on red note %q", depRed), staleAt}
	case tier == TierNone:
		if hasBrokenEvidence(n.Evidence) {
			return Verdict{Red, "referenced evidence does not resolve (broken ref)", staleAt}
		}
		return Verdict{Red, "no referenced observed/reported evidence", staleAt}
	case e.today.After(expiry):
		return Verdict{Red, "expired: today > last_verified + 2×window", staleAt}
	}

	// YELLOW — not red, but something keeps it below green.
	switch {
	case tier == TierReported || tier == TierReasoned:
		return Verdict{Yellow, "evidence is reported/reasoned (caps at yellow)", staleAt}
	case n.Check.Status != "passed": // tier is observed here
		return Verdict{Yellow, "observed evidence but check not passed", staleAt}
	case e.today.After(staleAt):
		return Verdict{Yellow, "stale: today > stale_at (but not expired)", staleAt}
	case depYellow != "":
		return Verdict{Yellow, fmt.Sprintf("depends on yellow note %q", depYellow), staleAt}
	}

	// GREEN — observed, check passed, fresh, every dependency green, no contradiction.
	return Verdict{Green, "observed evidence, check passed, fresh, deps green, no contradiction", staleAt}
}
