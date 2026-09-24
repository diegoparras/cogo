// Package agentdocs stores the agent-instruction files a user authors in the
// visor — AGENTS.md, CLAUDE.md, GEMINI.md, copilot-instructions.md, or any custom
// name. They live in the vault (.cogo/agents/) like every other side-state, so
// COGO stays standalone: one binary, no database. Each save appends to a small
// per-doc version history so the user can look back or restore.
package agentdocs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Known are the conventional filenames offered in the UI. Any safe name works.
var Known = []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", "copilot-instructions.md"}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.md$`)

// SafeName validates a doc name (no path separators, no traversal, no hidden
// files, must end in .md); "" if bad. A filename has no slashes — we reject
// rather than silently take the base, so the user isn't surprised.
func SafeName(name string) string {
	name = strings.TrimSpace(name)
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return ""
	}
	if !nameRe.MatchString(name) {
		return ""
	}
	return name
}

func dir(vault string) string     { return filepath.Join(vault, ".cogo", "agents") }
func histDir(vault string) string { return filepath.Join(dir(vault), ".history") }

// Meta is a listing entry: the doc name and when it was last saved.
type Meta struct {
	Name    string `json:"name"`
	Updated string `json:"updated"`
	Size    int    `json:"size"`
}

// Version is one saved revision of a doc.
type Version struct {
	Time    string `json:"time"`
	Content string `json:"content"`
}

// List returns the saved docs, newest first.
func List(vault string) []Meta {
	entries, err := os.ReadDir(dir(vault))
	if err != nil {
		return nil
	}
	var out []Meta
	for _, e := range entries {
		if e.IsDir() || SafeName(e.Name()) == "" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Meta{Name: e.Name(), Updated: info.ModTime().UTC().Format("2006-01-02 15:04"), Size: int(info.Size())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

// Load reads one doc's current content ("" if it doesn't exist yet).
func Load(vault, name string) (string, error) {
	n := SafeName(name)
	if n == "" {
		return "", fmt.Errorf("invalid doc name %q", name)
	}
	b, err := os.ReadFile(filepath.Join(dir(vault), n))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Save writes the doc and appends the new content to its version history
// (capped at the last 30 revisions). now is an RFC3339-ish timestamp string.
func Save(vault, name, content, now string) error {
	n := SafeName(name)
	if n == "" {
		return fmt.Errorf("invalid doc name %q", name)
	}
	if err := os.MkdirAll(histDir(vault), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir(vault), n), []byte(content), 0o644); err != nil {
		return err
	}
	vers := History(vault, n)
	vers = append(vers, Version{Time: now, Content: content})
	if len(vers) > 30 {
		vers = vers[len(vers)-30:]
	}
	var buf strings.Builder
	for _, v := range vers {
		b, _ := json.Marshal(v)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(histDir(vault), n+".jsonl"), []byte(buf.String()), 0o644)
}

// History returns the saved revisions of a doc, oldest first.
func History(vault, name string) []Version {
	n := SafeName(name)
	if n == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(histDir(vault), n+".jsonl"))
	if err != nil {
		return nil
	}
	var out []Version
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
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

// Delete removes a doc and its history.
func Delete(vault, name string) error {
	n := SafeName(name)
	if n == "" {
		return fmt.Errorf("invalid doc name %q", name)
	}
	_ = os.Remove(filepath.Join(histDir(vault), n+".jsonl"))
	err := os.Remove(filepath.Join(dir(vault), n))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
