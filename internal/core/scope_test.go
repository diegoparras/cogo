package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeConflict(t *testing.T) {
	scope := map[string]string{"os": "windows", "commit": "abc123"}
	// Same os → no conflict.
	if c := ScopeConflict(scope, map[string]string{"os": "Windows"}); len(c) != 0 {
		t.Errorf("case-insensitive match should not conflict, got %v", c)
	}
	// Different os → conflict on os only (commit not in env).
	c := ScopeConflict(scope, map[string]string{"os": "linux"})
	if len(c) != 1 || c["os"] != "linux" {
		t.Errorf("os conflict = %v, want {os:linux}", c)
	}
	// Empty env or scope → nil.
	if ScopeConflict(scope, nil) != nil || ScopeConflict(nil, map[string]string{"os": "x"}) != nil {
		t.Error("empty side should yield nil")
	}
}

func TestScopeAuthorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	n := &Note{
		ID: "n", Type: "bug", LastVerified: MustDate("2026-07-07"),
		Evidence: obs("x.go:1"), Check: Check{Status: "passed"},
		Author: "token:CI", Scope: map[string]string{"os": "windows", "go": "1.25"},
		Body: "## Claim\nthe build fails with npm ci",
	}
	path := filepath.Join(dir, "n.md")
	if err := WriteNoteFile(path, n); err != nil {
		t.Fatal(err)
	}
	got, err := ReadNoteFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Author != "token:CI" {
		t.Errorf("author round-trip: %q", got.Author)
	}
	if got.Scope["os"] != "windows" || got.Scope["go"] != "1.25" {
		t.Errorf("scope round-trip: %v", got.Scope)
	}
}

// TestPackFlagsScopeMismatch: a green note held on windows, consumed with env
// os=linux, must carry a loud ⚠ scope warning in the pack (not reach green blind).
func TestPackFlagsScopeMismatch(t *testing.T) {
	vault := map[string]*Note{
		"n": {
			ID: "n", Type: "constraint", LastVerified: MustDate("2026-07-07"),
			Evidence: obs("build.log:1"), Check: Check{Status: "passed"},
			Author: "token:winbox", Scope: map[string]string{"os": "windows"},
			Body: "## Claim\nthe build needs npm ci",
		},
	}
	// Matching env: no warning, but scope + author are shown.
	pk := BuildPack(vault, nil, PackOptions{Query: "build", Today: MustDate("2026-07-07"), Env: map[string]string{"os": "windows"}})
	if strings.Contains(pk.Markdown, "⚠ scope") {
		t.Errorf("same-os pack should not warn:\n%s", pk.Markdown)
	}
	if !strings.Contains(pk.Markdown, "by: token:winbox") || !strings.Contains(pk.Markdown, "scope: os=windows") {
		t.Errorf("pack should surface author+scope:\n%s", pk.Markdown)
	}
	// Conflicting env: loud warning.
	pk = BuildPack(vault, nil, PackOptions{Query: "build", Today: MustDate("2026-07-07"), Env: map[string]string{"os": "linux"}})
	if !strings.Contains(pk.Markdown, "⚠ scope") {
		t.Errorf("cross-os pack must warn:\n%s", pk.Markdown)
	}
}
