// Package history keeps a per-note trail: every time a note is written, one line
// is appended to .cogo/history/<id>.jsonl with the timestamp, the color it had,
// the reason, and its claim — so you can see WHEN and WHY a note flipped
// (green->red, etc.) instead of only its final state. Self-contained (no git).
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	b, err := json.Marshal(Version{Time: time.Now().UTC().Format(time.RFC3339), Color: color, Reason: reason, Claim: claim})
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
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
