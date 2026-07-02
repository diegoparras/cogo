package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/suasion"
)

// The Guard tab: the visor face of the autonomy engine. Paste any model's
// turn, keep your mandate persisted in the vault, get the radiography as
// structured JSON the SPA paints with the same confidence colors as notes.

func (s *Server) mandatePath() string { return suasion.MandatePath(s.dir) }

func (s *Server) handleMandate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m := suasion.LoadMandate(s.mandatePath())
		if m == nil {
			m = &suasion.Mandate{}
		}
		writeJSON(w, m)
	case http.MethodPost:
		var m suasion.Mandate
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := suasion.SaveMandate(s.mandatePath(), &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type guardReq struct {
	Turn       string `json:"turn"`
	Transcript []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	} `json:"transcript"`
	Goal     string   `json:"goal"`
	RedLines []string `json:"red_lines"`
	Steelman bool     `json:"steelman"`
}

type guardFinding struct {
	Technique   string            `json:"technique"`
	Name        string            `json:"name"`
	Color       string            `json:"color"`
	Reason      string            `json:"reason"`
	Evidence    string            `json:"evidence"`
	Receipts    []suasion.Receipt `json:"receipts,omitempty"`
	RedLine     string            `json:"red_line,omitempty"`
	Questions   []string          `json:"questions"`
	Move        string            `json:"move"`
	Inoculation string            `json:"inoculation"`
}

func (s *Server) handleGuard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eng, err := suasion.Default()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var in guardReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Turn) == "" {
		http.Error(w, "guard necesita el turno a analizar", http.StatusBadRequest)
		return
	}
	var transcript []suasion.Turn
	for _, t := range in.Transcript {
		transcript = append(transcript, suasion.Turn{Role: t.Role, Text: t.Text})
	}
	mandate := &suasion.Mandate{Goal: in.Goal, RedLines: in.RedLines}
	if in.Goal == "" && len(in.RedLines) == 0 {
		mandate = suasion.LoadMandate(s.mandatePath())
	}
	p := s.prov()
	rep := eng.AnalyzeWith(r.Context(), in.Turn, transcript, mandate, suasion.Opts{
		Tier1:    p,
		Tier2:    llm.StrongFromEnv(p),
		Steelman: in.Steelman,
	})

	findings := make([]guardFinding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		t := eng.Ontology.Get(f.TechniqueID)
		findings = append(findings, guardFinding{
			Technique: f.TechniqueID, Name: f.Name, Color: f.Color.String(),
			Reason: f.Reason, Evidence: f.Evidence, Receipts: f.Receipts,
			RedLine: f.RedLine, Questions: t.CriticalQuestions,
			Move:        strings.TrimSpace(t.Countermeasure.Move),
			Inoculation: strings.TrimSpace(t.Countermeasure.Inoculation),
		})
	}
	covered, total := eng.Coverage()
	writeJSON(w, map[string]any{
		"mode": rep.Mode, "overall": rep.Overall.String(), "reason": rep.Reason,
		"red_lines": rep.RedLines, "streak": rep.Trajectory.Streak,
		"findings": findings, "steelman": rep.Steelman, "steelman_note": rep.SteelmanNote,
		"covered": covered, "total": total,
	})
}
