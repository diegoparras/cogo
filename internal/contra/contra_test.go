package contra

import "testing"

func all(string) bool { return true }

func TestMergeDismissResolveAndPersist(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)

	s.Merge([]Found{{A: "a", B: "b", Reason: "clash 1"}, {A: "c", B: "d", Reason: "clash 2"}}, "2026-07-04", all)
	if got := len(s.OpenNoteSet()); got != 4 {
		t.Fatalf("4 note ids should be open (a,b,c,d), got %d", got)
	}

	// The a/b pair id, so we can dismiss it.
	ab := pairID("a", "b")
	if !s.Dismiss(ab) {
		t.Fatal("dismiss should find the a/b pair")
	}
	set := s.OpenNoteSet()
	if set["a"] || set["b"] {
		t.Error("dismissed pair must not paint red")
	}
	if !set["c"] || !set["d"] {
		t.Error("the other pair should still be open")
	}

	// Re-running lint must NOT re-open a dismissed pair.
	s.Merge([]Found{{A: "a", B: "b", Reason: "clash 1 again"}}, "2026-07-05", all)
	if s.OpenNoteSet()["a"] {
		t.Error("a dismissed pair must never be re-flagged")
	}

	// It survives a reload.
	if s2 := Open(dir); !s2.OpenNoteSet()["c"] {
		t.Error("open contradictions must persist across a reload")
	}

	// Resolve forgets the c/d pair entirely.
	if !s.Resolve(pairID("c", "d")) {
		t.Fatal("resolve should find the c/d pair")
	}
	if len(s.OpenNoteSet()) != 0 {
		t.Error("nothing should be open after resolving c/d")
	}

	// A pair whose note is gone gets pruned on the next merge.
	s.Merge([]Found{{A: "x", B: "y", Reason: "z"}}, "2026-07-06", func(id string) bool { return id != "y" })
	if s.OpenNoteSet()["x"] {
		t.Error("a contradiction whose note vanished must be pruned")
	}
}
