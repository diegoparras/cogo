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
	Score   int
}

// Search returns the notes relevant to the query, ordered by trust then
// relevance then id. limit <= 0 means no limit. It returns ids, colors and
// one-line summaries only — never bodies.
func Search(vault map[string]*Note, contradictions map[string]bool, query, project string, today Date, limit int) []SearchResult {
	verdicts := EvaluateVault(vault, contradictions, today)
	qterms := terms(query)

	var res []SearchResult
	for id, n := range vault {
		if project != "" && n.Project != project {
			continue
		}
		score := relevance(n, qterms)
		if len(qterms) > 0 && score == 0 {
			continue
		}
		res = append(res, SearchResult{
			ID:      id,
			Color:   verdicts[id].Color,
			Type:    n.Type,
			Project: n.Project,
			Summary: summarize(claimOf(n), 100),
			Score:   score,
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
