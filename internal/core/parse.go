package core

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseNote parses a note's Markdown (YAML frontmatter delimited by '---' lines
// plus a body) into a Note. The computed block, if present, is read too — but
// callers should always recompute the color, never trust a stored one.
func ParseNote(data []byte) (*Note, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return nil, fmt.Errorf("note has no frontmatter: must start with a '---' line")
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			closing = i
			break
		}
	}
	if closing == -1 {
		return nil, fmt.Errorf("frontmatter is not closed with a '---' line")
	}

	frontmatter := strings.Join(lines[1:closing], "\n")
	body := strings.Join(lines[closing+1:], "\n")

	var n Note
	if strings.TrimSpace(frontmatter) != "" {
		if err := yaml.Unmarshal([]byte(frontmatter), &n); err != nil {
			return nil, fmt.Errorf("frontmatter YAML: %w", err)
		}
	}
	if n.ID == "" {
		return nil, fmt.Errorf("note is missing required field: id")
	}
	n.Body = strings.Trim(body, "\n")
	return &n, nil
}

// Wire structs give the writer full control over field order and over omitting
// empty optionals, independently of how Note is read.
type fmInputs struct {
	ID           string     `yaml:"id"`
	Type         string     `yaml:"type"`
	Project      string     `yaml:"project,omitempty"`
	Evidence     []Evidence `yaml:"evidence,omitempty"`
	Check        *Check     `yaml:"check,omitempty"`
	LastVerified *Date      `yaml:"last_verified,omitempty"`
	DependsOn    []string   `yaml:"depends_on,omitempty"`
	Supersedes   string     `yaml:"supersedes,omitempty"`
	CausedBy     string     `yaml:"caused_by,omitempty"`
	Status       string     `yaml:"status,omitempty"`
}

type fmComputed struct {
	Confidence  string `yaml:"confidence"`
	StaleAt     *Date  `yaml:"stale_at,omitempty"`
	ColorReason string `yaml:"color_reason"`
}

// MarshalNote renders a Note back to Markdown: input frontmatter, then the
// computed block (only if a color was computed), clearly fenced as do-not-edit.
func MarshalNote(n *Note) ([]byte, error) {
	in := fmInputs{
		ID:         n.ID,
		Type:       n.Type,
		Project:    n.Project,
		Evidence:   n.Evidence,
		DependsOn:  n.DependsOn,
		Supersedes: n.Supersedes,
		CausedBy:   n.CausedBy,
		Status:     n.Status,
	}
	if n.Check != (Check{}) {
		c := n.Check
		in.Check = &c
	}
	if !n.LastVerified.IsZero() {
		d := n.LastVerified
		in.LastVerified = &d
	}

	inBytes, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(inBytes)

	if n.Confidence != "" {
		comp := fmComputed{Confidence: n.Confidence, ColorReason: n.ColorReason}
		if !n.StaleAt.IsZero() {
			d := n.StaleAt
			comp.StaleAt = &d
		}
		compBytes, err := yaml.Marshal(comp)
		if err != nil {
			return nil, err
		}
		buf.WriteString("\n# ---- computed by COGO · do not edit ----\n")
		buf.Write(compBytes)
	}
	buf.WriteString("---\n")

	if body := strings.Trim(n.Body, "\n"); body != "" {
		buf.WriteString("\n")
		buf.WriteString(body)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

// ReadNoteFile reads and parses a note from disk.
func ReadNoteFile(path string) (*Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n, err := ParseNote(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	n.Path = path
	return n, nil
}

// writeHook, if set, is called after every successful note write — the single
// choke point a face uses to record per-note history without core doing that
// I/O itself (it stays pure by default). See SetWriteHook.
var writeHook func(path string, n *Note)

// SetWriteHook installs a callback run after each WriteNoteFile. Pass nil to
// disable. Set once by the server; nil in tests keeps core deterministic.
func SetWriteHook(f func(path string, n *Note)) { writeHook = f }

// WriteNoteFile renders a note and writes it to disk.
func WriteNoteFile(path string, n *Note) error {
	data, err := MarshalNote(n)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if writeHook != nil {
		writeHook(path, n)
	}
	return nil
}
