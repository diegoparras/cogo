package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeNote(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const noteA = "---\nid: a\ntype: decision\nevidence:\n  - kind: file\n    ref: x.go\n---\nbody a\n"

func TestVaultCacheFreshnessAndDelete(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "a.md", noteA)
	writeNote(t, dir, "b.md", "---\nid: b\ntype: bug\n---\nbody b\n")

	c := NewVaultCache(dir)
	v, err := c.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 || v["a"] == nil || v["b"] == nil {
		t.Fatalf("first load: want a+b, got %v", keys(v))
	}

	// Rewrite b with a newer mtime and changed body -> next Load reflects it.
	time.Sleep(10 * time.Millisecond)
	writeNote(t, dir, "b.md", "---\nid: b\ntype: bug\n---\nCHANGED\n")
	// Force a distinct mtime even on coarse-resolution filesystems.
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(filepath.Join(dir, "b.md"), future, future)
	v, _ = c.Load()
	if v["b"].Body != "CHANGED" {
		t.Errorf("changed file not re-parsed: body=%q", v["b"].Body)
	}

	// Delete a -> it drops out and the internal map prunes it.
	if err := os.Remove(filepath.Join(dir, "a.md")); err != nil {
		t.Fatal(err)
	}
	v, _ = c.Load()
	if v["a"] != nil {
		t.Error("deleted note still present after Load")
	}
	if _, ok := c.fil[filepath.Join(dir, "a.md")]; ok {
		t.Error("cache did not prune deleted file")
	}
}

func TestVaultCacheCloneIsolation(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "a.md", noteA)
	c := NewVaultCache(dir)

	v1, _ := c.Load()
	// Mutate as ResolveEvidence/handlers would: set computed color + evidence status.
	v1["a"].Confidence = "green"
	v1["a"].Evidence[0].Status = "broken"

	// The file is unchanged, so the second Load reuses the cached template — but it
	// must hand back a pristine clone, not the mutated instance.
	v2, _ := c.Load()
	if v2["a"].Confidence == "green" {
		t.Error("mutation leaked into cached template (Confidence)")
	}
	if v2["a"].Evidence[0].Status == "broken" {
		t.Error("mutation leaked into cached template (Evidence slice shared)")
	}
	if v1["a"] == v2["a"] {
		t.Error("Load returned the same pointer twice; must be a fresh clone")
	}
}

func keys(m map[string]*Note) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
