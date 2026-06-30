package scrub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

func mockAnonimal(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/anonymize") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		red := strings.ReplaceAll(in["text"], "secret@mail.com", "[EMAIL]")
		red = strings.ReplaceAll(red, "PII", "[X]")
		_ = json.NewEncoder(w).Encode(map[string]any{"redacted_text": red})
	}))
}

func TestAnonimalScrub(t *testing.T) {
	ts := mockAnonimal(t)
	defer ts.Close()
	a := &Anonimal{URL: ts.URL}
	if !a.Enabled() {
		t.Fatal("should be enabled")
	}
	out, err := a.Scrub(context.Background(), "contact secret@mail.com now")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "secret@mail.com") || !strings.Contains(out, "[EMAIL]") {
		t.Errorf("not scrubbed: %q", out)
	}
}

func TestScrubNoteAllFields(t *testing.T) {
	ts := mockAnonimal(t)
	defer ts.Close()
	n := &core.Note{
		Body:     "claim with PII",
		Check:    core.Check{Test: "test PII connectivity"},
		Evidence: []core.Evidence{{Kind: "file_read", Ref: "log line with PII"}},
	}
	if err := Note(context.Background(), &Anonimal{URL: ts.URL}, n); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(n.Body, "PII") || strings.Contains(n.Check.Test, "PII") || strings.Contains(n.Evidence[0].Ref, "PII") {
		t.Errorf("PII leaked through scrub: %+v", n)
	}
}

func TestScrubOffByDefault(t *testing.T) {
	t.Setenv("ANONIMAL_URL", "")
	if FromEnv().Enabled() {
		t.Error("scrub must be off without ANONIMAL_URL")
	}
	// Noop leaves text untouched and never errors.
	n := &core.Note{Body: "raw secret@mail.com"}
	if err := Note(context.Background(), Noop{}, n); err != nil || n.Body != "raw secret@mail.com" {
		t.Errorf("noop changed the note: %q, %v", n.Body, err)
	}
}
