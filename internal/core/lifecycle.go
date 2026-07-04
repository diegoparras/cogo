package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	td := trashDir(dir)
	if err := os.MkdirAll(td, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(td, n.ID+".md")
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// TrashItem is a deleted note as shown in the trash view (id + a short claim).
type TrashItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Project string `json:"project"`
	Claim   string `json:"claim"`
}

func trashDir(dir string) string { return filepath.Join(dir, ".cogo", "trash") }

// ListTrash returns the notes currently in the trash (recoverable).
func ListTrash(dir string) []TrashItem {
	entries, err := os.ReadDir(trashDir(dir))
	if err != nil {
		return []TrashItem{}
	}
	out := []TrashItem{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		n, err := ReadNoteFile(filepath.Join(trashDir(dir), e.Name()))
		if err != nil {
			continue
		}
		out = append(out, TrashItem{ID: n.ID, Type: n.Type, Project: n.Project, Claim: summarize(claimOf(n), 160)})
	}
	return out
}

// RestoreTrash moves a trashed note back into the vault. It fails if a live note
// already uses that id (you'd clobber it).
func RestoreTrash(dir, id string) error {
	src := filepath.Join(trashDir(dir), id+".md")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("no such trashed note %q", id)
	}
	dst := filepath.Join(dir, id+".md")
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("ya existe una nota con id %q — no se puede restaurar encima", id)
	}
	return os.Rename(src, dst)
}

// PurgeTrash deletes a trashed note for good (no going back).
func PurgeTrash(dir, id string) error {
	if err := os.Remove(filepath.Join(trashDir(dir), id+".md")); err != nil {
		return fmt.Errorf("no such trashed note %q", id)
	}
	return nil
}
