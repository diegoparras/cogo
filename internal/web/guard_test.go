package web

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Guard tab flow: persist a mandate, then radiography a turn without
// re-declaring it — the saved mandate must kick in and escalate.
func TestGuardWithPersistedMandate(t *testing.T) {
	s := testServer(t)

	rec := call(s.handleMandate, "POST", "/api/mandate", map[string]any{
		"goal": "decidir sin apuro", "red_lines": []string{"no vendo mi casa"},
	})
	if rec.Code != 200 {
		t.Fatalf("save mandate: %d %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	json.Unmarshal(call(s.handleMandate, "GET", "/api/mandate", nil).Body.Bytes(), &got)
	if got["goal"] != "decidir sin apuro" {
		t.Fatalf("mandate round-trip lost the goal: %v", got)
	}

	rec = call(s.handleGuard, "POST", "/api/guard", map[string]any{
		"turn": "Vendé la casa ahora, antes de que sea tarde: cada semana perdés valor.",
	})
	if rec.Code != 200 {
		t.Fatalf("guard: %d %s", rec.Code, rec.Body.String())
	}
	var r struct {
		Mode     string `json:"mode"`
		Overall  string `json:"overall"`
		Findings []struct {
			Name        string   `json:"name"`
			Questions   []string `json:"questions"`
			Inoculation string   `json:"inoculation"`
		} `json:"findings"`
		RedLines []any `json:"red_lines"`
	}
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Mode != "mandato" {
		t.Errorf("mode = %q, want mandato (persisted mandate should load)", r.Mode)
	}
	if len(r.RedLines) == 0 {
		t.Error("the turn touches the persisted red line and it is not reported")
	}
	// A strong signal on a red line is a loud yellow, not a confident red
	// (pushing-across vs discussing is a judgment; only receipts earn red).
	if r.Overall != "yellow" && r.Overall != "red" {
		t.Errorf("overall = %q, want at least yellow (signal on the persisted red line)", r.Overall)
	}
	if len(r.Findings) == 0 || len(r.Findings[0].Questions) == 0 || r.Findings[0].Inoculation == "" {
		t.Errorf("findings must carry the inoculation payload: %+v", r.Findings)
	}

	// Without any mandate anywhere, the same turn degrades to informative.
	s2 := testServer(t)
	rec = call(s2.handleGuard, "POST", "/api/guard", map[string]any{
		"turn": "Vendé la casa ahora, antes de que sea tarde.",
	})
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Mode != "informativo" {
		t.Errorf("mode = %q, want informativo without a mandate", r.Mode)
	}
}

func TestGuardNeedsTurn(t *testing.T) {
	s := testServer(t)
	if rec := call(s.handleGuard, "POST", "/api/guard", map[string]any{"turn": "  "}); rec.Code != 400 {
		t.Errorf("empty turn should 400, got %d", rec.Code)
	}
	if rec := call(s.handleGuard, "GET", "/api/guard", nil); rec.Code != 405 {
		t.Errorf("GET should 405, got %d", rec.Code)
	}
}

func TestGuardSteelmanOffIsSaidNotSilent(t *testing.T) {
	s := testServer(t) // no provider configured
	rec := call(s.handleGuard, "POST", "/api/guard", map[string]any{
		"turn": "Vendé la casa ahora mismo.", "steelman": true,
	})
	if !strings.Contains(rec.Body.String(), "steelman solicitado pero no disponible") {
		t.Errorf("requested steelman without a model must be reported:\n%s", rec.Body.String())
	}
}
