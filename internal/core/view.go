package core

import (
	"sort"
	"time"
)

// NoteView is a note flattened for a face (web list, etc.): the computed color
// plus a short claim, ready to render. JSON-tagged for the web API.
type NoteView struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Project string `json:"project"`
	Color   string `json:"color"`
	Reason  string `json:"reason"`
	StaleAt string `json:"stale_at"`
	Claim   string `json:"claim"`
	State   string `json:"state,omitempty"`  // archived|retracted|superseded; empty = active
	Author  string `json:"author,omitempty"` // who captured it (multi-agent attribution)
	// Cuándo se verificó por última vez. La fecha de CREACIÓN no vive en la nota
	// (está en el historial), así que la completa la cara web: ver history.CreatedAt.
	Verified string `json:"verified,omitempty"`
	Created  string `json:"created,omitempty"`

	// Procedencia del respaldo: eje ORTOGONAL al color. El color dice cuánto
	// sostiene la evidencia; esto dice quién lo comprobó — "declared" si alguien
	// lo afirmó, "executed" si COGO corrió el check y vio su código de salida.
	// Se expone aparte justamente para no colapsar las dos preguntas en un color.
	Attested   string `json:"attested,omitempty"`
	AttestedBy string `json:"attested_by,omitempty"`

	// El eje de ORIGEN, solo en las normativas: quién eligió. Ver origen.go.
	Origin string `json:"origin,omitempty"`
	// La latencia: si la nota salió del camino, por qué, y hace cuánto que nadie
	// la mira. Se expone SIEMPRE, esté latente o no — el visor es el único lugar
	// donde una nota olvidada se puede volver a ver, y una que no se ve no se
	// puede recuperar. Ver latencia.go.
	Latent       bool   `json:"latent,omitempty"`
	LatentReason string `json:"latent_reason,omitempty"`
	UnusedDays   int    `json:"unused_days,omitempty"`
	Pinned       bool   `json:"pinned,omitempty"`
}

// Overview grades the whole vault and returns one NoteView per note, ordered
// red-first (what needs attention), then by id. Non-active notes (archived,
// retracted, superseded) are dropped unless includeArchived is set.
func Overview(vault map[string]*Note, contradictions map[string]bool, today Date, includeArchived bool) []NoteView {
	verdicts := EvaluateVault(vault, contradictions, today)
	state := Lifecycle(vault)
	lat := Latentes(vault, contradictions, today, time.Now())
	out := make([]NoteView, 0, len(vault))
	for id, n := range vault {
		if !includeArchived && state[id] != StateActive {
			continue
		}
		v := verdicts[id]
		out = append(out, NoteView{
			ID: id, Type: n.Type, Project: n.Project,
			Color: v.Color.String(), Reason: v.Reason,
			StaleAt: v.StaleAt.String(), Claim: summarize(claimOf(n), 200),
			State: stateTag(state, id), Author: n.Author, Verified: n.LastVerified.String(),
			Attested: attestedTag(n), AttestedBy: n.Check.AttestedBy,
			Origin:       origenTag(n),
			Latent:       lat[id].Latente,
			LatentReason: lat[id].Motivo,
			UnusedDays:   lat[id].DiasSinConsultar,
			Pinned:       n.Pinned,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if oi, oj := attentionOrder(out[i].Color), attentionOrder(out[j].Color); oi != oj {
			return oi < oj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// origenTag informa el origen solo en las normativas: en las demás la evidencia
// ya responde por la nota, y decirlo sería ruido.
func origenTag(n *Note) string {
	if !EsNormativa(n) {
		return ""
	}
	if o := OrigenDe(n); o != OrigenSinDeclarar {
		return string(o)
	}
	return "unrecorded"
}

// attestedTag informa la procedencia solo cuando hay algo que informar: una nota
// sin check no tiene qué respaldar, y decir "declarado" ahí sería ruido.
func attestedTag(n *Note) string {
	if n.Check.Status != "passed" {
		return ""
	}
	return n.Check.Attestation()
}

// attentionOrder puts the least trustworthy first: red, yellow, green, ungraded.
func attentionOrder(color string) int {
	switch color {
	case "red":
		return 0
	case "yellow":
		return 1
	case "green":
		return 2
	default:
		return 3
	}
}
