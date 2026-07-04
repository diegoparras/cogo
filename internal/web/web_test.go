package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/tokens"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	seed := &core.Note{
		ID: "redis", Type: "architecture", Project: "fisherboy",
		LastVerified: core.MustDate("2026-06-20"),
		Evidence:     []core.Evidence{{Kind: "file_read", Ref: "compose.yml:1"}},
		Check:        core.Check{Status: "passed"}, Body: "## Claim\nRedis at fisherboy-redis:6379.",
	}
	if err := core.WriteNoteFile(filepath.Join(dir, "redis.md"), seed); err != nil {
		t.Fatal(err)
	}
	return New(dir, func() core.Date { return core.MustDate("2026-06-29") }, tokens.Open(dir))
}

func call(h http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, target, rdr))
	return rec
}

func TestConfigAndNotes(t *testing.T) {
	s := testServer(t)

	var cfg map[string]any
	json.Unmarshal(call(s.handleConfig, "GET", "/api/config", nil).Body.Bytes(), &cfg)
	if cfg["count"].(float64) != 1 {
		t.Errorf("count = %v", cfg["count"])
	}
	if cfg["llm_configured"] != false || cfg["scrub_enabled"] != false {
		t.Errorf("accessories should be off by default: %v", cfg)
	}

	var notes []map[string]any
	json.Unmarshal(call(s.handleNotes, "GET", "/api/notes", nil).Body.Bytes(), &notes)
	if len(notes) != 1 || notes[0]["color"] != "green" {
		t.Errorf("notes = %v", notes)
	}
}

func TestPreviewDoesNotSave(t *testing.T) {
	s := testServer(t)
	draft := map[string]any{"type": "bug", "project": "demo", "body": "## Claim\nA pure guess."}

	var pv map[string]any
	json.Unmarshal(call(s.handlePreview, "POST", "/api/preview", draft).Body.Bytes(), &pv)
	if pv["color"] != "red" {
		t.Errorf("no-evidence draft should preview red, got %v", pv["color"])
	}
	// The vault still has only the seed — preview never persists.
	var notes []map[string]any
	json.Unmarshal(call(s.handleNotes, "GET", "/api/notes", nil).Body.Bytes(), &notes)
	if len(notes) != 1 {
		t.Errorf("preview must not save; count = %d", len(notes))
	}
}

func TestCaptureThenVerify(t *testing.T) {
	s := testServer(t)
	draft := map[string]any{
		"type": "bug", "project": "demo", "body": "## Claim\nThe worker reads config at boot.",
		"evidence": []map[string]string{{"kind": "file_read", "ref": "worker.go:12"}},
	}
	var cap map[string]any
	json.Unmarshal(call(s.handleCapture, "POST", "/api/capture", draft).Body.Bytes(), &cap)
	if cap["color"] != "yellow" { // observed evidence, check not_run
		t.Fatalf("capture color = %v", cap["color"])
	}
	id := cap["id"].(string)

	var notes []map[string]any
	json.Unmarshal(call(s.handleNotes, "GET", "/api/notes", nil).Body.Bytes(), &notes)
	if len(notes) != 2 {
		t.Fatalf("capture should persist; count = %d", len(notes))
	}

	var ver map[string]any
	json.Unmarshal(call(s.handleVerify, "POST", "/api/verify?id="+id, nil).Body.Bytes(), &ver)
	if ver["color"] != "green" { // check passed today -> green
		t.Errorf("verify should turn it green, got %v", ver["color"])
	}
}

func TestSettingsRoundTripNoKeyLeak(t *testing.T) {
	s := testServer(t)

	if rec := call(s.handleSettings, "GET", "/api/settings", nil); !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Errorf("should start off: %s", rec.Body.String())
	}

	call(s.handleSettings, "POST", "/api/settings", map[string]string{
		"base_url": "https://openrouter.ai/api/v1", "model": "deepseek/deepseek-chat", "api_key": "SECRET-XYZ",
	})

	rec := call(s.handleSettings, "GET", "/api/settings", nil)
	body := rec.Body.String()
	if !strings.Contains(body, `"has_key":true`) || !strings.Contains(body, `"configured":true`) {
		t.Errorf("settings not persisted: %s", body)
	}
	if strings.Contains(body, "SECRET-XYZ") {
		t.Error("API key leaked through GET /api/settings")
	}
}

func TestLintRunsDeterministic(t *testing.T) {
	s := testServer(t)
	var r map[string]any
	json.Unmarshal(call(s.handleLint, "POST", "/api/lint", nil).Body.Bytes(), &r)
	if r["llm_used"] != false {
		t.Errorf("no model configured, llm_used should be false: %v", r)
	}
}
