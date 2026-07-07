package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRefClassifies(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "api.go")
	if err := os.WriteFile(real, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		ref, root, want string
	}{
		{"api.go:40", dir, EvResolved},                        // repo-relative, exists (root set)
		{"api.go:40", "", EvUnchecked},                        // relative, no root -> can't check
		{"gone.go:1", dir, EvBroken},                          // repo-relative, missing
		{"api.go:40 — connect OK", dir, EvResolved},           // trailing prose is stripped
		{"a log line: connect OK to redis", dir, EvUnchecked}, // prose, not a path
		{"https://example.com/x", dir, EvUnchecked},           // URL -> offline, unchecked
		{"file://.../index.html line 3-9", dir, EvUnchecked},  // elided path -> unchecked
		{filepath.Join(dir, "api.go"), "", EvResolved},        // absolute, exists
		{filepath.Join(dir, "nope.go") + ":9", "", EvBroken},  // absolute, missing
	}
	for _, c := range cases {
		if got := resolveRef(c.ref, c.root); got != c.want {
			t.Errorf("resolveRef(%q, %q) = %q, want %q", c.ref, c.root, got, c.want)
		}
	}
}

func TestBrokenEvidenceSinksGreen(t *testing.T) {
	dir := t.TempDir()
	vault := map[string]*Note{
		"n": {
			ID: "n", Type: "bug", LastVerified: MustDate("2026-06-20"),
			Evidence: []Evidence{{Kind: "file_read", Ref: filepath.Join(dir, "ghost.go") + ":10"}},
			Check:    Check{Test: "x", Status: "passed"},
			Body:     "## Claim\nsomething",
		},
	}
	// Before resolving, the non-empty ref counts -> green.
	if c := Evaluate(vault["n"], vault, nil, MustDate("2026-06-29")).Color; c != Green {
		t.Fatalf("pre-resolve want green, got %s", c)
	}
	// After resolving, the broken file ref no longer holds up the color.
	ResolveEvidence(vault, EvidenceRoots{})
	v := Evaluate(vault["n"], vault, nil, MustDate("2026-06-29"))
	if v.Color != Red {
		t.Errorf("broken evidence should sink to red, got %s", v.Color)
	}
	if v.Reason != "referenced evidence does not resolve (broken ref)" {
		t.Errorf("reason should name the broken ref, got %q", v.Reason)
	}
}
