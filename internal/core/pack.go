package core

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Pack is a budgeted, color-aware context digest for one query. It is what an
// agent consumes: green notes as fact, yellow flagged as probable, and red
// physically quarantined into a "do not rely" section — never mixed in as fact.
type Pack struct {
	Query     string
	Markdown  string
	Tokens    int // estimated tokens of the included note blocks
	RawTokens int // tokens it would cost to read every matching note in full
	Greens    int
	Yellows   int
	Reds      int
	Mistakes  int
	Dropped   int // notes left out to stay under budget
	// Incluidas son los ids que efectivamente entraron. Es lo que permite
	// registrar el uso: una nota entró en un pack = alguien la consumió. Sin
	// esto, olvidar tendría que decidirse por la edad. Ver internal/uso.
	Incluidas []string
	// Latentes es cuántas quedaron fuera del camino por olvido. Se informa
	// aparte de Dropped porque son cosas distintas: una salió por presupuesto y
	// puede volver mañana; la otra salió porque nadie la usa.
	Latentes int
}

// PackOptions parameterizes a pack. Budget is an approximate token ceiling on
// the note content (0 = unlimited). Today is injected for deterministic color.
type PackOptions struct {
	Query   string
	Project string
	Budget  int
	Today   Date
	// Env is the consuming agent's environment (os, commit, runtime…). When a
	// note's Scope declares a conflicting condition, the pack flags it so a claim
	// isn't trusted blind on a machine it wasn't made for. Optional.
	Env map[string]string
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
	// La latencia se resuelve sobre el vault ENTERO, antes de filtrar por
	// proyecto: una de sus condiciones es que nadie dependa de la nota, y un
	// dependiente puede estar en otro proyecto.
	latentes := Latentes(vault, contradictions, opts.Today, time.Now())

	var pool []*Note
	for id, n := range vault {
		if hidden[id] || (opts.Project != "" && n.Project != opts.Project) {
			continue // archived/retracted/superseded never feed an agent's context
		}
		if latentes[id].Latente {
			continue // vencida, sin dependientes y sin consultar: fuera del camino
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
		block := renderBlock(n, v, opts.Env)
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
	var greens, yellows, mistakes, reds, brechas []string
	var incluidas []string
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
		incluidas = append(incluidas, c.n.ID)
		switch c.v.Color {
		case Green:
			greens = append(greens, c.block)
		case Yellow:
			yellows = append(yellows, c.block)
		case Ungraded:
			// Las brechas también son `ungraded`, pero no son lo mismo que un
			// error registrado: una dice qué NO se sabe, la otra qué salió mal.
			// Mezclarlas escondería justamente lo que hay que ver.
			if EsBrecha(c.n) {
				brechas = append(brechas, c.block)
			} else {
				mistakes = append(mistakes, c.block)
			}
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
	if len(brechas) > 0 {
		fmt.Fprintf(&b, " · **%d** open questions", len(brechas))
	}
	if dropped > 0 {
		fmt.Fprintf(&b, " · %d omitted (budget)", dropped)
	}
	b.WriteString("\n")
	// The point of the pack: consume this instead of re-reading the notes in full.
	if rawTokens > running && running > 0 {
		fmt.Fprintf(&b, "> _~%d tokens vs ~%d reading these notes in full — %.0f%% less._\n",
			running, rawTokens, 100*(1-float64(running)/float64(rawTokens)))
	}

	// "treat as fact" prometía más de lo que el sistema podía sostener: hasta que
	// exista el runner, todo check pasado es una declaración. El encabezado ahora
	// dice qué respalda cada nota y deja que el agente lea la procedencia de cada
	// línea de check.
	writeSection(&b, "Supported — evidence holds; check the attestation before acting on it", greens)
	writeSection(&b, "Probable — likely, not certain", yellows)
	writeSection(&b, "Do not repeat — past mistakes", mistakes)
	writeSection(&b, "Assumptions — DO NOT RELY", reds)
	// Las preguntas abiertas van ÚLTIMAS y en su propia sección. No son
	// conocimiento degradado: son la ausencia de conocimiento, dicha en voz
	// alta. Un agente que llega hasta acá sabe qué NO puede averiguar leyendo el
	// vault — y eso es información, no un hueco.
	writeSection(&b, "Open questions — nobody knows this yet; do not guess", brechas)

	if len(greens)+len(yellows)+len(mistakes)+len(reds)+len(brechas) == 0 {
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
		Incluidas: incluidas,
		Latentes:  contarLatentes(latentes),
	}
}

func contarLatentes(l map[string]Latencia) int {
	n := 0
	for _, x := range l {
		if x.Latente {
			n++
		}
	}
	return n
}

// BuildConstraints renders the load-bearing memory an agent must NOT lose across
// a context compaction: the verified (green) decisions and constraints, terse.
// These are the "active constraints still binding" that compaction silently
// erodes; re-injecting them re-anchors the agent. "" if there are none.
// project "" includes the whole vault; a project filters to that project's
// load-bearing notes, so an agent re-anchors on its repo's rules without the
// noise of the others.
func BuildConstraints(vault map[string]*Note, contradictions map[string]bool, today Date, project string) string {
	verdicts := EvaluateVault(vault, contradictions, today)
	hidden := Hidden(vault)
	var lines []string
	for id, n := range vault {
		if hidden[id] || verdicts[id].Color != Green {
			continue // only verified, live notes are load-bearing
		}
		if project != "" && n.Project != project {
			continue
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
func renderBlock(n *Note, v Verdict, env map[string]string) string {
	if EsBrecha(n) {
		return renderBrecha(n)
	}
	claim := claimOf(n)
	meta := scopeMeta(n, env)
	switch v.Color {
	case Green:
		return fmt.Sprintf("### %s · %s\n%s\n- check: %s\n%s\n", n.ID, n.Type, claim, checkLine(n), meta)
	case Yellow:
		return fmt.Sprintf("### %s · %s\n%s\n- check: %s\n- caveat: %s\n%s\n", n.ID, n.Type, claim, checkLine(n), v.Reason, meta)
	case Ungraded:
		return fmt.Sprintf("- **%s**: %s\n", n.ID, claim)
	default: // Red
		return fmt.Sprintf("- **%s**: %s — _unverified: %s_\n", n.ID, claim, v.Reason)
	}
}

// lineaOrigen dice quién originó una decisión o una restricción, cuando eso no
// se puede dar por sentado.
//
// Solo aparece en las normativas, y solo cuando hay algo que advertir. Un `bug`
// o un `runbook` describen el mundo y la evidencia ya responde por ellos; una
// decisión afirma que alguien ELIGIÓ, y ahí saber quién es la mitad del dato. El
// agente que lee "proposed by an agent" sabe que puede discutirlo; sin esa línea
// lo trataría como algo ya resuelto.
func lineaOrigen(n *Note) string {
	switch {
	case !EsNormativa(n):
		return ""
	case EsPropuesta(n):
		return "\n- origin: **proposed by an agent** — no human chose this; it is open to revision"
	case OrigenDe(n) == OrigenHumano:
		return "\n- origin: decided by a human"
	case OrigenDe(n) == OrigenInstrumento:
		return "\n- origin: measured, not chosen"
	default:
		return "\n- origin: unrecorded — nobody knows who decided this"
	}
}

// renderBrecha muestra una pregunta abierta. No se renderiza como una nota
// degradada porque no lo es: lo que importa acá no es un claim con su respaldo
// sino la pregunta, qué está trabando, y qué ya se intentó — para que el próximo
// no choque contra la misma pared.
func renderBrecha(n *Note) string {
	q := strings.TrimSpace(n.Question)
	if q == "" {
		q = claimOf(n)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n**%s**\n", n.ID, q)
	if k := len(n.Blocks); k > 0 {
		fmt.Fprintf(&b, "- blocks %d decision(s): %s\n", k, strings.Join(n.Blocks, ", "))
	}
	if c := strings.TrimSpace(n.CostToResolve); c != "" {
		fmt.Fprintf(&b, "- cost to resolve: %s\n", c)
	}
	for _, a := range n.Attempted {
		fmt.Fprintf(&b, "- already tried: %s\n", a)
	}
	b.WriteString("\n")
	return b.String()
}

// scopeMeta renders a note's author and scope for a pack block, warning loudly
// when the scope conflicts with the consuming agent's env — the fix for a claim
// true on one machine silently reaching green on another.
func scopeMeta(n *Note, env map[string]string) string {
	var lines []string
	if n.Author != "" {
		lines = append(lines, "- by: "+n.Author)
	}
	if len(n.Scope) > 0 {
		if conflict := ScopeConflict(n.Scope, env); len(conflict) > 0 {
			lines = append(lines, "- ⚠ scope: held under "+ScopeString(n.Scope)+" — your env differs ("+ScopeString(conflict)+"); verify before relying on this here")
		} else {
			lines = append(lines, "- scope: "+ScopeString(n.Scope))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func checkLine(n *Note) string {
	if strings.TrimSpace(n.Check.Test) == "" {
		return "—"
	}
	status := n.Check.Status
	if status == "" {
		status = "not_run"
	}
	// La procedencia viaja junto al estado del check. Un agente que va a apoyar
	// una acción sobre esta nota necesita distinguir un check que se ejecutó de
	// uno que alguien afirmó: son dos grados de respaldo muy distintos y hasta
	// ahora llegaban escritos igual.
	if status == "passed" {
		return fmt.Sprintf("%s (passed, %s)", n.Check.Test, n.Check.Attestation())
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
