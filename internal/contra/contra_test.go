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

	// Resolve keeps the c/d pair on record as resolved: nothing is red, but the
	// count of what COGO caught does not shrink.
	if !s.ResolveOn(pairID("c", "d"), "2026-07-05") {
		t.Fatal("resolve should find the c/d pair")
	}
	if len(s.OpenNoteSet()) != 0 {
		t.Error("nothing should be open after resolving c/d")
	}
	if r := s.Resumen(); r.Abiertas != 0 || r.Resueltas != 1 || r.Descartadas != 1 {
		t.Errorf("resumen: %+v", r)
	}
	for _, c := range s.List() {
		if c.ID == pairID("c", "d") && (c.Status != StatusResolved || c.Resolved != "2026-07-05") {
			t.Errorf("resolved pair must carry its date: %+v", c)
		}
	}
	// If lint finds the same pair again, it re-opens (a dismissed one would not).
	s.Merge([]Found{{A: "c", B: "d", Reason: "still clashes"}}, "2026-07-06", all)
	if !s.OpenNoteSet()["c"] {
		t.Error("a resolved pair that comes back must re-open")
	}
	if r := s.Resumen(); r.Abiertas != 1 || r.Resueltas != 0 {
		t.Errorf("resumen after re-open: %+v", r)
	}
	if !s.ResolveOn(pairID("c", "d"), "2026-07-06") {
		t.Fatal("resolve again")
	}

	// A pair whose note is gone gets pruned on the next merge.
	s.Merge([]Found{{A: "x", B: "y", Reason: "z"}}, "2026-07-06", func(id string) bool { return id != "y" })
	if s.OpenNoteSet()["x"] {
		t.Error("a contradiction whose note vanished must be pruned")
	}
}

func TestForNote(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	s.Merge([]Found{{A: "a", B: "b", Reason: "a vs b"}, {A: "a", B: "c", Reason: "a vs c"}, {A: "d", B: "e", Reason: "d vs e"}}, "2026-07-04", all)

	got := s.ForNote("a")
	if len(got) != 2 {
		t.Fatalf("note a is in 2 contradictions, got %d", len(got))
	}
	others := map[string]string{got[0].Other: got[0].Reason, got[1].Other: got[1].Reason}
	if others["b"] != "a vs b" || others["c"] != "a vs c" {
		t.Errorf("ForNote must name the OTHER side and its reason, got %+v", got)
	}
	// Seen from the other side too.
	if bs := s.ForNote("b"); len(bs) != 1 || bs[0].Other != "a" {
		t.Errorf("b should point back at a, got %+v", bs)
	}
	// A note in no contradiction gets nothing.
	if len(s.ForNote("z")) != 0 {
		t.Error("a note in no contradiction should return no conflicts")
	}
	// Dismissed contradictions drop out of the trace.
	s.Dismiss(pairID("a", "b"))
	if got := s.ForNote("a"); len(got) != 1 || got[0].Other != "c" {
		t.Errorf("dismissed pair must not appear in ForNote, got %+v", got)
	}
}
