package lint

import (
	"context"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/llm"
)

var today = core.MustDate("2026-06-29")

// fakeProvider always answers the same thing — no network, deterministic test.
type fakeProvider struct{ reply string }

func (fakeProvider) Available() bool                                    { return true }
func (fakeProvider) Name() string                                       { return "fake" }
func (f fakeProvider) Complete(context.Context, string) (string, error) { return f.reply, nil }

func TestLintBrokenDep(t *testing.T) {
	vault := map[string]*core.Note{
		"a": {ID: "a", Type: "bug", Project: "p", LastVerified: today,
			Evidence: []core.Evidence{{Kind: "file_read", Ref: "x"}}, Check: core.Check{Status: "passed"},
			DependsOn: []string{"ghost"}, Body: "## Claim\nA."},
	}
	r := Run(context.Background(), vault, today, llm.Noop{})
	if len(r.Issues) != 1 || r.Issues[0].Kind != "broken_dep" {
		t.Fatalf("expected one broken_dep issue, got %+v", r.Issues)
	}
	if r.LLMUsed {
		t.Error("Noop provider must not be used")
	}
}

func TestLintContradiction(t *testing.T) {
	vault := map[string]*core.Note{
		"redis-up":   {ID: "redis-up", Type: "bug", Project: "fisherboy", LastVerified: today, Body: "## Claim\nThe redis hostname resolves correctly from the worker."},
		"redis-down": {ID: "redis-down", Type: "bug", Project: "fisherboy", LastVerified: today, Body: "## Claim\nThe redis hostname does not resolve from the worker."},
	}
	r := Run(context.Background(), vault, today, fakeProvider{"YES: one says it resolves, the other says it does not"})

	if !r.LLMUsed {
		t.Fatal("LLM should have been used")
	}
	if r.CandidatePairs != 1 || r.PairsChecked != 1 {
		t.Fatalf("expected one candidate pair checked, got candidates=%d checked=%d", r.CandidatePairs, r.PairsChecked)
	}
	c := r.Contradictions()
	if !c["redis-up"] || !c["redis-down"] {
		t.Errorf("both notes should be marked contradicting: %v", c)
	}

	// And the contradiction set, fed to the color engine, turns them red.
	v := core.Evaluate(vault["redis-up"], vault, c, today)
	if v.Color != core.Red {
		t.Errorf("a contradicting note must color red, got %s", v.Color)
	}
}
