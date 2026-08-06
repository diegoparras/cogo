// Package scrub is the OPTIONAL capture sanitizer — an accessory, not core.
// Off by default (standalone needs nothing). When ANONIMAL_URL is set, every
// captured note runs through Anonimal so secrets/PII never enter the vault. It
// uses the same legacy /anonymize contract Escriba and Fisherboy use.
package scrub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/core"
)

type Scrubber interface {
	Enabled() bool
	Scrub(ctx context.Context, text string) (string, error)
}

// Noop is the default: scrubbing off, text passes through untouched.
type Noop struct{}

func (Noop) Enabled() bool                                        { return false }
func (Noop) Scrub(_ context.Context, text string) (string, error) { return text, nil }

// Anonimal calls the suite's Anonimal service: POST {text} -> {redacted_text}.
type Anonimal struct {
	URL    string
	Token  string
	Client *http.Client
}

func (a *Anonimal) Enabled() bool { return a.URL != "" }

func (a *Anonimal) Scrub(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return text, nil
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL+"/anonymize", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.Token != "" {
		req.Header.Set("X-Anonimal-Token", a.Token) // suite service token (require_auth)
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anonimal http %d", resp.StatusCode)
	}
	var out struct {
		RedactedText string `json:"redacted_text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.RedactedText == "" {
		return text, nil // nothing detected
	}
	return out.RedactedText, nil
}

// FromEnv builds a scrubber from ANONIMAL_URL (+ optional ANONIMAL_TOKEN).
// Unset -> Noop (off).
func FromEnv() Scrubber {
	url := strings.TrimRight(os.Getenv("ANONIMAL_URL"), "/")
	if url == "" {
		return Noop{}
	}
	return &Anonimal{URL: url, Token: os.Getenv("ANONIMAL_TOKEN")}
}

// Note scrubs every persisted text field of a note in place. A scrub error
// fails CLOSED — the caller must not persist the note (privacy-first).
func Note(ctx context.Context, s Scrubber, n *core.Note) error {
	if !s.Enabled() {
		return nil
	}
	var err error
	if n.Body, err = s.Scrub(ctx, n.Body); err != nil {
		return err
	}
	if n.Check.Test != "" {
		if n.Check.Test, err = s.Scrub(ctx, n.Check.Test); err != nil {
			return err
		}
	}
	for i := range n.Evidence {
		if n.Evidence[i].Ref == "" {
			continue
		}
		if n.Evidence[i].Ref, err = s.Scrub(ctx, n.Evidence[i].Ref); err != nil {
			return err
		}
	}
	return nil
}
