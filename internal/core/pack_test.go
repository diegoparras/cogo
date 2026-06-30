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
			LastVerified: MustDate("2026-06-20"), Evidence: []Evidence{{"doc", "README#worker"}},
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
