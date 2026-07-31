// Package history keeps a per-note trail: every time a note is written, one line
// is appended to .cogo/history/<id>.jsonl with the timestamp, the color it had,
// the reason, and its claim — so you can see WHEN and WHY a note flipped
// (green->red, etc.) instead of only its final state. Self-contained (no git).
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Version is one recorded state of a note.
type Version struct {
	Time   string `json:"time"`   // RFC3339 UTC
	Color  string `json:"color"`  // green|yellow|red|ungraded
	Reason string `json:"reason"` // why it had that color
	Claim  string `json:"claim"`  // the note's headline claim at that point
}

var mu sync.Mutex

func fileFor(vault, id string) string {
	return filepath.Join(vault, ".cogo", "history", id+".jsonl")
}

// Record appends a version (best-effort — history must never break a write).
func Record(vault, id, color, reason, claim string) {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Join(vault, ".cogo", "history"), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(fileFor(vault, id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	// Sub-second precision so a recall cursor never collides with a same-second
	// write (which strict "after" comparison would then drop forever).
	b, err := json.Marshal(Version{Time: time.Now().UTC().Format(time.RFC3339Nano), Color: color, Reason: reason, Claim: claim})
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
}

// Change is the latest recorded state of a note, tagged with its id — what
// `recall(since:…)` reports as the delta another agent needs to catch up.
type Change struct {
	ID     string `json:"id"`
	Time   string `json:"time"`
	Color  string `json:"color"`
	Reason string `json:"reason"`
	Claim  string `json:"claim"`
}

// ChangedSince returns the latest recorded version of every note whose most
// recent write is strictly newer than `since` (RFC3339 UTC), newest first. It is
// the cursor that turns the vault from a shared archive into a channel: machine B
// asks "what changed since I last looked" instead of re-reading everything. An
// empty or unparseable `since` means "everything" — every note's latest version.
func ChangedSince(vault, since string) []Change {
	mu.Lock()
	defer mu.Unlock()
	histDir := filepath.Join(vault, ".cogo", "history")
	entries, err := os.ReadDir(histDir)
	if err != nil {
		return []Change{}
	}
	cut, hasCut := parseTime(since) // no/bad cursor => everything passes
	type item struct {
		c Change
		t time.Time
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		last, ok := lastVersion(filepath.Join(histDir, e.Name()))
		if !ok {
			continue
		}
		t, ok := parseTime(last.Time)
		if !ok || (hasCut && !t.After(cut)) {
			continue
		}
		items = append(items, item{
			c: Change{
				ID:    strings.TrimSuffix(e.Name(), ".jsonl"),
				Time:  last.Time,
				Color: last.Color, Reason: last.Reason, Claim: last.Claim,
			},
			t: t,
		})
	}
	// Sort by parsed instant (robust to mixed second/nanosecond precision), newest first.
	sort.Slice(items, func(i, j int) bool { return items[i].t.After(items[j].t) })
	out := make([]Change, len(items))
	for i, it := range items {
		out[i] = it.c
	}
	return out
}

// parseTime accepts RFC3339 with or without fractional seconds.
func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// lastVersion returns the final recorded version in a history file. Lock-free:
// callers already hold mu.
func lastVersion(path string) (Version, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Version{}, false
	}
	var last Version
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var v Version
		if json.Unmarshal([]byte(line), &v) == nil {
			last, found = v, true
		}
	}
	return last, found
}

// Load returns a note's versions, oldest first ([] if none).
func Load(vault, id string) []Version {
	mu.Lock()
	defer mu.Unlock()
	b, err := os.ReadFile(fileFor(vault, id))
	if err != nil {
		return []Version{}
	}
	out := []Version{}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var v Version
		if json.Unmarshal([]byte(line), &v) == nil {
			out = append(out, v)
		}
	}
	return out
}
