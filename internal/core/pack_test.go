package core

import (
	"strings"
	"testing"
)

func packVault() map[string]*Note {
	return map[string]*Note{
		"green-redis": {
			ID: "green-redis", Type: "bug", Project: "fisherboy",
			LastVerified: MustDate("2026-06-20"), Evidence: obs("api.go:40"),
			Check: Check{Test: "connect to redis:6379", Status: "passed"},
			Body:  "## Claim\nThe redis hostname resolves correctly from the api.",
		},
		"yellow-worker": {
			ID: "yellow-worker", Type: "bug", Project: "fisherboy",
			LastVerified: MustDate("2026-06-20"), Evidence: []Evidence{{Kind: "doc", Ref: "README#worker"}},
			Body: "## Claim\nThe worker reads its config at boot.",
		},
		"red-guess": {
			ID: "red-guess", Type: "bug", Project: "fisherboy",
			LastVerified: MustDate("2026-06-20"), // no evidence -> red
			Body:         "## Claim\nThe queue is probably backed up.",
		},
		"mistake-cookie": {
			ID: "mistake-cookie", Type: "mistake", Project: "fisherboy",
			Body: "## Claim\nClearing a cookie needs the same flags as the set.",
		},
		"other-proj": {
			ID: "other-proj", Type: "bug", Project: "selega",
			LastVerified: MustDate("2026-06-20"), Evidence: obs("z.go:1"),
			Check: Check{Status: "passed"}, Body: "## Claim\nUnrelated note.",
		},
	}
}

var packToday = MustDate("2026-06-29")

func TestPackQuarantinesRed(t *testing.T) {
	p := BuildPack(packVault(), nil, PackOptions{Project: "fisherboy", Today: packToday})
	md := p.Markdown

	if p.Greens != 1 || p.Yellows != 1 || p.Reds != 1 || p.Mistakes != 1 {
		t.Fatalf("counts: %d green, %d yellow, %d red, %d mistakes\n%s", p.Greens, p.Yellows, p.Reds, p.Mistakes, md)
	}
	if !strings.Contains(md, "DO NOT RELY") {
		t.Errorf("red section header missing:\n%s", md)
	}

	// The red note's claim must live under the assumptions section, never in
	// "Verified" — that is the whole point of degrading red.
	verifiedIdx := strings.Index(md, "## Verified")
	assumptionsIdx := strings.Index(md, "## Assumptions")
	redIdx := strings.Index(md, "red-guess")
	if !(verifiedIdx < assumptionsIdx && redIdx > assumptionsIdx) {
		t.Errorf("red note not quarantined after assumptions header (verified=%d assumptions=%d red=%d)\n%s",
			verifiedIdx, assumptionsIdx, redIdx, md)
	}

	// Section order: verified before probable before assumptions.
	if !(verifiedIdx < strings.Index(md, "## Probable") && strings.Index(md, "## Probable") < assumptionsIdx) {
		t.Errorf("sections out of order:\n%s", md)
	}
}

func TestPackProjectFilter(t *testing.T) {
	p := BuildPack(packVault(), nil, PackOptions{Project: "fisherboy", Today: packToday})
	if strings.Contains(p.Markdown, "other-proj") {
		t.Errorf("project filter leaked a selega note:\n%s", p.Markdown)
	}
}

func TestPackQueryRelevance(t *testing.T) {
	// Only the green note mentions "redis".
	p := BuildPack(packVault(), nil, PackOptions{Query: "redis", Today: packToday})
	if p.Greens != 1 || p.Yellows != 0 || p.Reds != 0 {
		t.Fatalf("query should select only the redis note: %+v\n%s", p, p.Markdown)
	}
	if !strings.Contains(p.Markdown, "green-redis") {
		t.Errorf("relevant note missing:\n%s", p.Markdown)
	}
}

func TestPackBudgetDropsNotes(t *testing.T) {
	const budget = 40 // enough for one green block (~29 tokens), not for the rest
	full := BuildPack(packVault(), nil, PackOptions{Project: "fisherboy", Today: packToday})
	tight := BuildPack(packVault(), nil, PackOptions{Project: "fisherboy", Today: packToday, Budget: budget})

	if tight.Dropped == 0 {
		t.Errorf("tight budget dropped nothing (tokens=%d)", tight.Tokens)
	}
	if tight.Tokens > budget {
		t.Errorf("budget exceeded: %d > %d", tight.Tokens, budget)
	}
	included := tight.Greens + tight.Yellows + tight.Reds + tight.Mistakes
	fullIncluded := full.Greens + full.Yellows + full.Reds + full.Mistakes
	if included >= fullIncluded {
		t.Errorf("tight budget did not reduce notes: %d vs %d", included, fullIncluded)
	}
	// Trust-monotonic: the green is kept; the less-trusted notes are dropped.
	if tight.Greens == 0 {
		t.Errorf("budget dropped the green, keeping less-trusted notes:\n%s", tight.Markdown)
	}
	if tight.Reds != 0 {
		t.Errorf("a red was kept while higher-trust notes were dropped:\n%s", tight.Markdown)
	}
}

// TestPackTokenSavings: with a long-bodied note the pack summarizes and reports
// a real saving; with only short notes it does NOT claim one (the pack adds
// structure/color there — the value isn't fewer tokens). Honesty either way.
func TestPackTokenSavings(t *testing.T) {
	long := map[string]*Note{
		"big": {ID: "big", Type: "runbook", LastVerified: packToday,
			Evidence: obs("run.sh:1"), Check: Check{Test: "run it", Status: "passed"},
			Body: "## Claim\nDeploy steps.\n\n" + strings.Repeat("step: do a thing, then another thing, carefully. ", 60)},
	}
	p := BuildPack(long, nil, PackOptions{Today: packToday})
	if p.RawTokens <= p.Tokens {
		t.Fatalf("a long note should compress: pack=%d raw=%d", p.Tokens, p.RawTokens)
	}
	if !strings.Contains(p.Markdown, "less.") {
		t.Errorf("digest should report the saving, got:\n%s", firstLine(p.Markdown))
	}

	// Short notes: no false saving claim.
	sp := BuildPack(packVault(), nil, PackOptions{Today: packToday})
	if strings.Contains(sp.Markdown, "less.") {
		t.Errorf("short notes should NOT claim a token saving (raw=%d pack=%d)", sp.RawTokens, sp.Tokens)
	}
}

// TestSearchBM25RareTerm: a rare, discriminating term outranks a common one —
// the note carrying the rare term should come first.
func TestSearchBM25RareTerm(t *testing.T) {
	vault := map[string]*Note{
		"a": {ID: "a", Type: "bug", LastVerified: packToday, Body: "## Claim\nthe worker uses redis widely, redis redis redis"},
		"b": {ID: "b", Type: "bug", LastVerified: packToday, Body: "## Claim\nthe kafka consumer lags"},
		"c": {ID: "c", Type: "bug", LastVerified: packToday, Body: "## Claim\nredis is common here too, redis"},
	}
	// "kafka" is rare (1 doc), "redis" is common (2 docs). Query both; the rare
	// term must lift note b above the redis-heavy notes.
	res := Search(vault, nil, "kafka redis", "", packToday, 0, false)
	if len(res) == 0 || res[0].ID != "b" {
		t.Errorf("rare-term note should rank first, got order %v", ids(res))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
func ids(res []SearchResult) []string {
	out := make([]string, len(res))
	for i, r := range res {
		out[i] = r.ID
	}
	return out
}
