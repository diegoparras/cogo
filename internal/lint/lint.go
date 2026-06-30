// Package lint is the OPTIONAL maintenance pass: deterministic checks (broken
// links, stale notes) plus, only when a model is configured, contradiction
// detection. It lives outside core/ — core stays deterministic. Contradictions
// it finds feed the color engine's clause #1 (an open contradiction -> red).
package lint

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/llm"
)

// Issue is one finding. Kind is broken_dep | stale | contradiction.
type Issue struct {
	Kind string   `json:"kind"`
	IDs  []string `json:"ids"`
	Msg  string   `json:"msg"`
}

// Report is the result of a lint pass.
type Report struct {
	Issues         []Issue
	LLMUsed        bool
	CandidatePairs int // pairs worth asking the model about
	PairsChecked   int // pairs actually asked (bounded)
}

// Contradictions returns the set of note ids touched by a contradiction, ready
// to hand to core.Evaluate so those notes go red.
func (r Report) Contradictions() map[string]bool {
	set := map[string]bool{}
	for _, is := range r.Issues {
		if is.Kind == "contradiction" {
			for _, id := range is.IDs {
				set[id] = true
			}
		}
	}
	return set
}

// maxPairs bounds the model cost of a single lint pass. If more candidate pairs
// exist, the overflow is reported (CandidatePairs > PairsChecked), never hidden.
const maxPairs = 24

// Run performs the deterministic checks always, and contradiction detection only
// if the provider is available.
func Run(ctx context.Context, vault map[string]*core.Note, today core.Date, p llm.Provider) Report {
	r := Report{}
	verdicts := core.EvaluateVault(vault, nil, today)

	ids := make([]string, 0, len(vault))
	for id := range vault {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		n := vault[id]
		for _, d := range n.DependsOn {
			if _, ok := vault[d]; !ok {
				r.Issues = append(r.Issues, Issue{"broken_dep", []string{id}, fmt.Sprintf("%s depends_on missing note %q", id, d)})
			}
		}
		if v := verdicts[id]; !v.StaleAt.IsZero() && today.After(v.StaleAt) {
			r.Issues = append(r.Issues, Issue{"stale", []string{id}, fmt.Sprintf("%s is stale (since %s)", id, v.StaleAt)})
		}
	}

	if p.Available() {
		r.LLMUsed = true
		r.detectContradictions(ctx, vault, ids, p)
	}

	sort.Slice(r.Issues, func(i, j int) bool {
		if r.Issues[i].Kind != r.Issues[j].Kind {
			return r.Issues[i].Kind < r.Issues[j].Kind
		}
		return strings.Join(r.Issues[i].IDs, ",") < strings.Join(r.Issues[j].IDs, ",")
	})
	return r
}

func (r *Report) detectContradictions(ctx context.Context, vault map[string]*core.Note, ids []string, p llm.Provider) {
	termCache := map[string]map[string]bool{}
	termsOf := func(id string) map[string]bool {
		if t, ok := termCache[id]; ok {
			return t
		}
		t := terms(core.Claim(vault[id]))
		termCache[id] = t
		return t
	}

	type pair struct{ a, b string }
	var pairs []pair
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, b := vault[ids[i]], vault[ids[j]]
			if a.Project != b.Project {
				continue // only compare notes in the same project
			}
			if overlap(termsOf(ids[i]), termsOf(ids[j])) < 2 {
				continue // cheap gate: must share some vocabulary to possibly clash
			}
			pairs = append(pairs, pair{ids[i], ids[j]})
		}
	}
	r.CandidatePairs = len(pairs)
	if len(pairs) > maxPairs {
		pairs = pairs[:maxPairs]
	}

	for _, pr := range pairs {
		r.PairsChecked++
		if yes, why := ask(ctx, p, vault[pr.a], vault[pr.b]); yes {
			r.Issues = append(r.Issues, Issue{"contradiction", []string{pr.a, pr.b}, why})
		}
	}
}

// ask poses one bounded yes/no question. The model proposes; it never writes.
func ask(ctx context.Context, p llm.Provider, a, b *core.Note) (bool, string) {
	prompt := "You check two project notes for a DIRECT contradiction (both cannot be true at the same time).\n\n" +
		"Note A (" + a.ID + "): " + core.Claim(a) + "\n" +
		"Note B (" + b.ID + "): " + core.Claim(b) + "\n\n" +
		"Reply EXACTLY 'YES: <short reason>' if they directly contradict, or 'NO' otherwise."
	out, err := p.Complete(ctx, prompt)
	if err != nil {
		return false, ""
	}
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(strings.ToUpper(out), "YES") {
		return false, ""
	}
	why := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(out, "YES"), "yes"))
	why = strings.TrimSpace(strings.TrimPrefix(why, ":"))
	return true, fmt.Sprintf("%s ⇄ %s: %s", a.ID, b.ID, why)
}

func terms(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(f) >= 4 {
			out[f] = true
		}
	}
	return out
}

func overlap(a, b map[string]bool) int {
	n := 0
	for t := range a {
		if b[t] {
			n++
		}
	}
	return n
}
