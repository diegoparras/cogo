package core

import "sort"

// SearchResult is one hit: id, computed color and a one-line summary — no body.
// Bounded output keeps the agent's context cheap (see §7).
type SearchResult struct {
	ID      string
	Color   Color
	Type    string
	Project string
	Summary string
	Score   float64
	State   string // archived|retracted|superseded; empty = active
}

// Search returns the notes relevant to the query, ordered by trust then
// relevance then id. limit <= 0 means no limit. It returns ids, colors and
// one-line summaries only — never bodies. Non-active notes are excluded unless
// includeArchived is set.
func Search(vault map[string]*Note, contradictions map[string]bool, query, project string, today Date, limit int, includeArchived bool) []SearchResult {
	verdicts := EvaluateVault(vault, contradictions, today)
	state := Lifecycle(vault)
	qterms := terms(query)

	var pool []*Note
	for id, n := range vault {
		if !includeArchived && state[id] != StateActive {
			continue
		}
		if project != "" && n.Project != project {
			continue
		}
		pool = append(pool, n)
	}
	rk := newRanker(pool, qterms)

	var res []SearchResult
	for _, n := range pool {
		score := rk.score(n, qterms, today)
		if len(qterms) > 0 && score <= 0 {
			continue
		}
		res = append(res, SearchResult{
			ID:      n.ID,
			Color:   verdicts[n.ID].Color,
			Type:    n.Type,
			Project: n.Project,
			Summary: summarize(claimOf(n), 100),
			Score:   score,
			State:   stateTag(state, n.ID),
		})
	}

	sort.Slice(res, func(i, j int) bool {
		if ri, rj := rank(res[i].Color), rank(res[j].Color); ri != rj {
			return ri < rj
		}
		if res[i].Score != res[j].Score {
			return res[i].Score > res[j].Score
		}
		return res[i].ID < res[j].ID
	})

	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res
}
