package core

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Pack is a budgeted, color-aware context digest for one query. It is what an
// agent consumes: green notes as fact, yellow flagged as probable, and red
// physically quarantined into a "do not rely" section — never mixed in as fact.
type Pack struct {
	Query    string
	Markdown string
	Tokens   int // estimated tokens of the included note blocks
	RawTokens int // tokens it would cost to read every matching note in full
	Greens   int
	Yellows  int
	Reds     int
	Mistakes int
	Dropped  int // notes left out to stay under budget
}

// PackOptions parameterizes a pack. Budget is an approximate token ceiling on
// the note content (0 = unlimited). Today is injected for deterministic color.
type PackOptions struct {
	Query   string
	Project string
	Budget  int
	Today   Date
}

// BuildPack grades the whole vault, selects the notes relevant to the query,
// orders them by trust then relevance, and renders a deterministic digest that
// fits the token budget.
func BuildPack(vault map[string]*Note, contradictions map[string]bool, opts PackOptions) Pack {
	verdicts := EvaluateVault(vault, contradictions, opts.Today)
	hidden := Hidden(vault)
	qterms := terms(opts.Query)

	// First pass: the candidate pool (visible, project-filtered). Its stats feed
	// the BM25 ranker, and its full bodies are the "read it all" baseline used for
	// the token-savings figure.
	var pool []*Note
	for id, n := range vault {
		if hidden[id] || (opts.Project != "" && n.Project != opts.Project) {
			continue // archived/retracted/superseded never feed an agent's context
		}
		pool = append(pool, n)
	}
	rk := newRanker(pool, qterms)

	type cand struct {
		n     *Note
		v     Verdict
		score float64
		block string
		toks  int
	}
	var cands []cand
	rawTokens := 0 // cost of reading the RELEVANT notes in full (the pack's alternative)
	for _, n := range pool {
		score := rk.score(n, qterms, opts.Today)
		if len(qterms) > 0 && score <= 0 {
			continue // a query was given but nothing matched
		}
		v := verdicts[n.ID]
		block := renderBlock(n, v)
		cands = append(cands, cand{n: n, v: v, score: score, block: block, toks: estimateTokens(block)})
		rawTokens += estimateTokens(n.Body)
	}

	// Most trustworthy first (green, yellow, mistakes, red), then most relevant
	// (BM25 + recency), then by id so the output is stable for prompt caching.
	sort.Slice(cands, func(i, j int) bool {
		if ri, rj := rank(cands[i].v.Color), rank(cands[j].v.Color); ri != rj {
			return ri < rj
		}
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].n.ID < cands[j].n.ID
	})

	// Trust-monotonic budgeting: once a note of some trust tier is dropped, no
	// less-trusted note is included — we never spend the budget on a red while a
	// green was left out. Within a tier we may skip a big note and keep a later
	// smaller one.
	var greens, yellows, mistakes, reds []string
	running, dropped := 0, 0
	droppedRank := 99
	for _, c := range cands {
		r := rank(c.v.Color)
		if r > droppedRank || (opts.Budget > 0 && running+c.toks > opts.Budget) {
			dropped++
			if r < droppedRank {
				droppedRank = r
			}
			continue
		}
		running += c.toks
		switch c.v.Color {
		case Green:
			greens = append(greens, c.block)
		case Yellow:
			yellows = append(yellows, c.block)
		case Ungraded:
			mistakes = append(mistakes, c.block)
		default:
			reds = append(reds, c.block)
		}
	}

	var b strings.Builder
	if opts.Query != "" {
		fmt.Fprintf(&b, "# Context pack — %q\n", opts.Query)
	} else {
		b.WriteString("# Context pack — all notes\n")
	}
	fmt.Fprintf(&b, "> **%d** verified · **%d** probable · **%d** assumptions · **%d** mistakes · ~**%d** tokens",
		len(greens), len(yellows), len(reds), len(mistakes), running)
	if dropped > 0 {
		fmt.Fprintf(&b, " · %d omitted (budget)", dropped)
	}
	b.WriteString("\n")
	// The point of the pack: consume this instead of re-reading the notes in full.
	if rawTokens > running && running > 0 {
		fmt.Fprintf(&b, "> _~%d tokens vs ~%d reading these notes in full — %.0f%% less._\n",
			running, rawTokens, 100*(1-float64(running)/float64(rawTokens)))
	}

	writeSection(&b, "Verified — treat as fact", greens)
	writeSection(&b, "Probable — likely, not certain", yellows)
	writeSection(&b, "Do not repeat — past mistakes", mistakes)
	writeSection(&b, "Assumptions — DO NOT RELY", reds)

	if len(greens)+len(yellows)+len(mistakes)+len(reds) == 0 {
		b.WriteString("\n_No matching notes._\n")
	}

	return Pack{
		Query:     opts.Query,
		Markdown:  b.String(),
		Tokens:    running,
		RawTokens: rawTokens,
		Greens:    len(greens),
		Yellows:   len(yellows),
		Reds:      len(reds),
		Mistakes:  len(mistakes),
		Dropped:   dropped,
	}
}

// BuildConstraints renders the load-bearing memory an agent must NOT lose across
// a context compaction: the verified (green) decisions and constraints, terse.
// These are the "active constraints still binding" that compaction silently
// erodes; re-injecting them re-anchors the agent. "" if there are none.
func BuildConstraints(vault map[string]*Note, contradictions map[string]bool, today Date) string {
	verdicts := EvaluateVault(vault, contradictions, today)
	hidden := Hidden(vault)
	var lines []string
	for id, n := range vault {
		if hidden[id] || verdicts[id].Color != Green {
			continue // only verified, live notes are load-bearing
		}
		if n.Type != "decision" && n.Type != "constraint" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- **%s** (%s): %s", id, n.Type, claimOf(n)))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func writeSection(b *strings.Builder, title string, blocks []string) {
	if len(blocks) == 0 {
		return
	}
	b.WriteString("\n## ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, bl := range blocks {
		b.WriteString(bl)
	}
}

// renderBlock formats one note for its color. Green/yellow get a heading with
// the claim and its minimal check; mistakes and reds are terse list items, and
// reds carry the reason they can't be trusted.
func renderBlock(n *Note, v Verdict) string {
	claim := claimOf(n)
	switch v.Color {
	case Green:
		return fmt.Sprintf("### %s · %s\n%s\n- check: %s\n\n", n.ID, n.Type, claim, checkLine(n))
	case Yellow:
		return fmt.Sprintf("### %s · %s\n%s\n- check: %s\n- caveat: %s\n\n", n.ID, n.Type, claim, checkLine(n), v.Reason)
	case Ungraded:
		return fmt.Sprintf("- **%s**: %s\n", n.ID, claim)
	default: // Red
		return fmt.Sprintf("- **%s**: %s — _unverified: %s_\n", n.ID, claim, v.Reason)
	}
}

func checkLine(n *Note) string {
	if strings.TrimSpace(n.Check.Test) == "" {
		return "—"
	}
	status := n.Check.Status
	if status == "" {
		status = "not_run"
	}
	return fmt.Sprintf("%s (%s)", n.Check.Test, status)
}

// rank orders colors by trust for the pack: green first, red last.
func rank(c Color) int {
	switch c {
	case Green:
		return 0
	case Yellow:
		return 1
	case Ungraded:
		return 2
	default:
		return 3
	}
}

// terms splits a query into lowercase tokens of length >= 2.
func terms(q string) []string {
	fields := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	out := fields[:0]
	for _, f := range fields {
		if len(f) >= 2 {
			out = append(out, f)
		}
	}
	return out
}

// ranker scores notes against the query with Okapi BM25 (term saturation + IDF
// + length normalization), so a note doesn't win just by repeating a word and a
// rare, discriminating term counts more than a common one. Deterministic — no
// model, no index to persist. A small recency term breaks ties toward fresher
// notes; an id hit is boosted. With no query it orders newest-first.
type ranker struct {
	idf    map[string]float64
	avgLen float64
	k1, b  float64
}

func rankTokens(n *Note) []string { return terms(n.ID + " " + n.Project + " " + n.Type + " " + n.Body) }

func newRanker(pool []*Note, qterms []string) *ranker {
	r := &ranker{idf: map[string]float64{}, k1: 1.5, b: 0.75}
	nDocs := float64(len(pool))
	if nDocs == 0 {
		return r
	}
	df := map[string]int{}
	var totalLen float64
	for _, n := range pool {
		toks := rankTokens(n)
		totalLen += float64(len(toks))
		seen := map[string]bool{}
		for _, t := range toks {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}
	r.avgLen = totalLen / nDocs
	if r.avgLen == 0 {
		r.avgLen = 1
	}
	for _, t := range qterms {
		d := float64(df[t])
		r.idf[t] = math.Log(1 + (nDocs-d+0.5)/(d+0.5)) // BM25 IDF, always > 0
	}
	return r
}

func (r *ranker) score(n *Note, qterms []string, today Date) float64 {
	if len(qterms) == 0 {
		return recencyBonus(n, today) // no query → newest-first
	}
	toks := rankTokens(n)
	tf := map[string]int{}
	for _, t := range toks {
		tf[t]++
	}
	docLen := float64(len(toks))
	idLow := strings.ToLower(n.ID)
	var s float64
	for _, t := range qterms {
		f := float64(tf[t])
		if f == 0 {
			continue
		}
		idf := r.idf[t]
		s += idf * (f * (r.k1 + 1) / (f + r.k1*(1-r.b+r.b*docLen/r.avgLen)))
		if strings.Contains(idLow, t) {
			s += 2 * idf // a hit in the id is worth more than one in the body
		}
	}
	if s == 0 {
		return 0
	}
	return s + 0.15*recencyBonus(n, today) // small recency tiebreak, never dominates
}

// recencyBonus decays from 1 (verified today) toward 0 over ~half a year.
func recencyBonus(n *Note, today Date) float64 {
	if n.LastVerified.IsZero() {
		return 0
	}
	days := today.DaysSince(n.LastVerified)
	if days < 0 {
		days = 0
	}
	return 1.0 / (1.0 + float64(days)/180.0)
}

// Claim returns a note's headline claim, summarized — exported for faces and
// the optional lint/llm layer.
func Claim(n *Note) string { return claimOf(n) }

// claimOf pulls a short claim for the digest: the "## Claim" section if present,
// else the first paragraph.
func claimOf(n *Note) string {
	if s := section(n.Body, "claim"); s != "" {
		return summarize(s, 280)
	}
	return summarize(firstParagraph(n.Body), 280)
}

// section returns the text under a "## <heading>" line until the next heading.
func section(body, heading string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "#") {
			continue
		}
		h := strings.ToLower(strings.TrimSpace(strings.TrimLeft(t, "# ")))
		if start == -1 {
			if h == heading {
				start = i + 1
			}
			continue
		}
		return strings.Join(lines[start:i], "\n")
	}
	if start != -1 {
		return strings.Join(lines[start:], "\n")
	}
	return ""
}

// firstParagraph collapses the first run of non-heading, non-blank lines.
func firstParagraph(body string) string {
	var b strings.Builder
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "#") || t == "" {
			if b.Len() > 0 {
				break
			}
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t)
	}
	return b.String()
}

// summarize collapses whitespace and truncates to maxRunes with an ellipsis.
func summarize(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return strings.TrimSpace(string(r[:maxRunes])) + "…"
}

// estimateTokens is a deterministic ~chars/4 heuristic. Good enough for a live
// counter and budget; a real tokenizer is not worth the dependency in v1.
func estimateTokens(s string) int {
	return (len([]rune(s)) + 3) / 4
}
