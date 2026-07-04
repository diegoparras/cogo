package core

import (
	"os"
	"path/filepath"
)

// The lifecycle axis is orthogonal to color. Color answers "how much can I trust
// this?"; lifecycle answers "is this note still live?". A note can be green
// (well-evidenced) yet superseded, or red yet active. Only ACTIVE notes appear
// in the default views (pack, graph, overview, search); the rest stay in the
// vault — deps and edges still resolve to them, and they can be restored —
// because COGO never throws away the epistemic trail.
const (
	StateActive     = "active"     // the default; empty Status means active
	StateArchived   = "archived"   // put away by hand: done / obsolete, kept for the record
	StateRetracted  = "retracted"  // withdrawn by hand: "this was wrong", kept as a tombstone
	StateSuperseded = "superseded" // computed: an active note supersedes this one
)

// Lifecycle returns the effective state of every note in the vault. A stored
// Status of archived/retracted wins. Otherwise a note is "superseded" when some
// note that is itself not archived/retracted declares `supersedes: <id>` — this
// finally makes the supersedes edge bury the old note instead of leaving it in
// the graph. Everything else is "active". The result depends only on stored
// fields, so it is deterministic regardless of map iteration order.
func Lifecycle(vault map[string]*Note) map[string]string {
	state := make(map[string]string, len(vault))
	for id, n := range vault {
		switch n.Status {
		case StateArchived, StateRetracted:
			state[id] = n.Status
		default:
			state[id] = StateActive
		}
	}
	for _, n := range vault {
		// A note that was itself put away doesn't get to bury its target.
		if n.Status == StateArchived || n.Status == StateRetracted || n.Supersedes == "" {
			continue
		}
		if _, ok := vault[n.Supersedes]; ok && state[n.Supersedes] == StateActive {
			state[n.Supersedes] = StateSuperseded
		}
	}
	return state
}

// Hidden is the set of note IDs to drop from default views: everything whose
// effective state is not active. They remain in the vault (so dependencies and
// edges resolve) and are restorable.
func Hidden(vault map[string]*Note) map[string]bool {
	h := make(map[string]bool)
	for id, st := range Lifecycle(vault) {
		if st != StateActive {
			h[id] = true
		}
	}
	return h
}

// stateTag returns the state for display, blanking "active" so it can be
// omitted from JSON with omitempty.
func stateTag(state map[string]string, id string) string {
	if st := state[id]; st != StateActive {
		return st
	}
	return ""
}

// TrashNote moves a note's file out of the vault into .cogo/trash instead of
// hard-deleting it, so a "delete" is still recoverable by hand even when the
// vault isn't under git. .cogo is skipped by LoadVault, so a trashed note leaves
// every view. Returns the trash path. Prefer archiving; this is for real garbage.
func TrashNote(dir string, n *Note) (string, error) {
	src := n.Path
	if src == "" {
		src = filepath.Join(dir, n.ID+".md")
	}
	trashDir := filepath.Join(dir, ".cogo", "trash")
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(trashDir, n.ID+".md")
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}
