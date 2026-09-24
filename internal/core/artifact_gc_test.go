package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReferencedArtifactsGC covers the refcount that keeps a deduplicated store
// safe to garbage-collect: a blob cited by two notes survives until BOTH are
// gone; a blob cited by nobody drops.
func TestReferencedArtifactsGC(t *testing.T) {
	dir := t.TempDir()
	const shaA, shaB = "aaaaaaaa", "bbbbbbbb"
	mk := func(id string, refs ...string) *Note {
		var ev []Evidence
		for _, r := range refs {
			ev = append(ev, Evidence{Kind: "command_output", Ref: r})
		}
		return &Note{ID: id, Type: "bug", LastVerified: MustDate("2026-07-07"), Evidence: ev, Check: Check{Status: "passed"}, Body: "## Claim\nx"}
	}
	for _, n := range []*Note{
		mk("n1", ArtifactRef(shaA)), // n1 and n2 share shaA
		mk("n2", ArtifactRef(shaA)),
		mk("n3", ArtifactRef(shaB)),
	} {
		if err := WriteNoteFile(filepath.Join(dir, n.ID+".md"), n); err != nil {
			t.Fatal(err)
		}
	}

	// ArtifactRefs pulls only artifact refs, ignoring ordinary ones.
	if got := ArtifactRefs(mk("x", ArtifactRef(shaA), "file.go:12")); len(got) != 1 || got[0] != shaA {
		t.Fatalf("ArtifactRefs = %v, want [%s]", got, shaA)
	}

	if keep := ReferencedArtifacts(dir); !keep[shaA] || !keep[shaB] {
		t.Fatalf("all present: keep=%v, want both", keep)
	}

	// Purge n1 → shaA still kept (n2 cites it).
	if err := os.Remove(filepath.Join(dir, "n1.md")); err != nil {
		t.Fatal(err)
	}
	if keep := ReferencedArtifacts(dir); !keep[shaA] {
		t.Fatal("shaA dropped while n2 still cites it — would corrupt n2's evidence")
	}

	// Purge n2 too → shaA orphaned; shaB still held by n3.
	if err := os.Remove(filepath.Join(dir, "n2.md")); err != nil {
		t.Fatal(err)
	}
	keep := ReferencedArtifacts(dir)
	if keep[shaA] {
		t.Fatal("shaA still kept after its last citer is gone (leak)")
	}
	if !keep[shaB] {
		t.Fatal("shaB dropped while n3 still cites it")
	}
}
