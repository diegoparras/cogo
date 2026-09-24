package suasion

import "fmt"

// Stage 5, deterministic: the damage of a human jailbreak rarely lives in one
// message — it accumulates. The trajectory counts lexical signals on every
// prior MODEL turn and detects sustained pressure; when the streak lands on a
// declared red line, that is gradualism made mechanical.

// TrajectoryPoint is one model turn's pressure reading.
type TrajectoryPoint struct {
	TurnIndex int  `json:"turn_index"` // index into the transcript; -1 = the turn under analysis
	Signals   int  `json:"signals"`
	RedLine   bool `json:"red_line"`
}

// Trajectory is the longitudinal reading, oldest first, ending at the current
// turn. Streak counts the trailing consecutive model turns carrying signals.
type Trajectory struct {
	Points []TrajectoryPoint `json:"points,omitempty"`
	Streak int               `json:"streak"`
}

// Sustained pressure threshold: three model turns in a row with signals.
const streakThreshold = 3

func (e *Engine) computeTrajectory(turn string, transcript []Turn, mandate *Mandate, currentSignals int) Trajectory {
	var tr Trajectory
	for i, t := range transcript {
		if t.Role != RoleModel {
			continue
		}
		tr.Points = append(tr.Points, TrajectoryPoint{
			TurnIndex: i,
			Signals:   len(e.lexiconFindings(t.Text)),
			RedLine:   len(mandate.redLineHits(t.Text)) > 0,
		})
	}
	tr.Points = append(tr.Points, TrajectoryPoint{
		TurnIndex: -1,
		Signals:   currentSignals,
		RedLine:   len(mandate.redLineHits(turn)) > 0,
	})
	for i := len(tr.Points) - 1; i >= 0 && tr.Points[i].Signals > 0; i-- {
		tr.Streak++
	}
	return tr
}

// gradualismFinding turns a sustained streak into a finding on the ontology's
// coercion.gradualism — detected mechanically, no model involved.
func (e *Engine) gradualismFinding(tr Trajectory) *Finding {
	if tr.Streak < streakThreshold {
		return nil
	}
	t := e.Ontology.Get("coercion.gradualism")
	return &Finding{
		TechniqueID: t.ID, Name: t.Name, Axes: t.Axes, Severity: t.Severity,
		Detector: "trajectory",
		Evidence: fmt.Sprintf("%d turnos consecutivos del modelo con señales de presión", tr.Streak),
	}
}
