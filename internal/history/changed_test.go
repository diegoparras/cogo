package history

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChangedSince verifies the recall cursor: only notes written after the
// cursor come back, newest first, and an empty cursor returns everything.
func TestChangedSince(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".cogo", "history")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two notes, hand-written with fixed timestamps (Record uses time.Now, which
	// we can't control, so we seed the jsonl directly).
	write := func(id string, lines ...string) {
		if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(join(lines)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("old",
		`{"time":"2026-07-01T10:00:00Z","color":"yellow","reason":"seen","claim":"old claim"}`)
	write("fresh",
		`{"time":"2026-07-05T09:00:00Z","color":"yellow","reason":"seen","claim":"fresh v1"}`,
		`{"time":"2026-07-07T12:00:00Z","color":"green","reason":"passed","claim":"fresh v2"}`)

	// Cursor between the two: only "fresh" (its latest is 07-07) should return.
	got := ChangedSince(vault, "2026-07-03T00:00:00Z")
	if len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("since 07-03: want [fresh], got %+v", got)
	}
	if got[0].Color != "green" || got[0].Claim != "fresh v2" {
		t.Errorf("want the LATEST version of fresh (green/v2), got %s/%q", got[0].Color, got[0].Claim)
	}

	// Cursor at/after the newest: nothing.
	if got := ChangedSince(vault, "2026-07-08T00:00:00Z"); len(got) != 0 {
		t.Errorf("since 07-08: want none, got %+v", got)
	}

	// Empty cursor: everything, newest first.
	all := ChangedSince(vault, "")
	if len(all) != 2 || all[0].ID != "fresh" || all[1].ID != "old" {
		t.Fatalf("empty cursor: want [fresh, old], got %+v", all)
	}

	// No history dir at all: empty, no panic.
	if got := ChangedSince(t.TempDir(), ""); len(got) != 0 {
		t.Errorf("missing history dir: want none, got %+v", got)
	}
}

func join(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}
