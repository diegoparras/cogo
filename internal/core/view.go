package core

import "sort"

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
	State   string `json:"state,omitempty"` // archived|retracted|superseded; empty = active
}

// Overview grades the whole vault and returns one NoteView per note, ordered
// red-first (what needs attention), then by id. Non-active notes (archived,
// retracted, superseded) are dropped unless includeArchived is set.
func Overview(vault map[string]*Note, contradictions map[string]bool, today Date, includeArchived bool) []NoteView {
	verdicts := EvaluateVault(vault, contradictions, today)
	state := Lifecycle(vault)
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
			State: stateTag(state, id),
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
