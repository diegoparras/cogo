package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEvidenceDrift: a green note whose cited file changes after verification
// drops to yellow with the drift reason; re-stamping restores it.
func TestEvidenceDrift(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "api.go")
	if err := os.WriteFile(file, []byte("package x // v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := SingleRoot(dir)

	n := &Note{
		ID: "n", Type: "bug", LastVerified: MustDate("2026-07-04"),
		Evidence: []Evidence{{Kind: "file_read", Ref: "api.go"}},
		Check:    Check{Test: "read it", Status: "passed"},
		Body:     "## Claim\nthe api does X",
	}
	vault := map[string]*Note{"n": n}
	today := MustDate("2026-07-05")

	// Stamp the baseline (as verify would), then color: green.
	StampEvidenceHashes(n, roots)
	if n.Evidence[0].Hash == "" {
		t.Fatal("verify should stamp a hash for a resolvable file ref")
	}
	ResolveEvidence(vault, roots)
	if c := Evaluate(n, vault, nil, today).Color; c != Green {
		t.Fatalf("fresh verified note should be green, got %s", c)
	}

	// The cited file changes → drift → yellow with the drift reason.
	if err := os.WriteFile(file, []byte("package x // v2 CHANGED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ResolveEvidence(vault, roots)
	if n.Evidence[0].Status != EvDrifted {
		t.Errorf("changed file should mark evidence drifted, got %q", n.Evidence[0].Status)
	}
	v := Evaluate(n, vault, nil, today)
	if v.Color != Yellow {
		t.Errorf("drifted evidence should drop green to yellow, got %s", v.Color)
	}
	if v.Reason == "" || v.Reason == "observed evidence, check passed, fresh, deps green, no contradiction" {
		t.Errorf("reason should name the drift, got %q", v.Reason)
	}

	// Re-verifying (re-stamp) against the new content clears the drift.
	StampEvidenceHashes(n, roots)
	ResolveEvidence(vault, roots)
	if c := Evaluate(n, vault, nil, today).Color; c != Green {
		t.Errorf("re-verified note should be green again, got %s", c)
	}
}
