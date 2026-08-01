package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/diegoparras/cogo/internal/history"
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

	notes, _, _ := notesOf(t, s, "")
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
	notes, _, _ := notesOf(t, s, "")
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

	notes, _, _ := notesOf(t, s, "")
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

// notesOf lee la respuesta paginada de /api/notes ({notes, total, facets}).
func notesOf(t *testing.T, s *Server, query string) ([]map[string]any, int, map[string]any) {
	t.Helper()
	var out struct {
		Notes  []map[string]any `json:"notes"`
		Total  int              `json:"total"`
		Facets map[string]any   `json:"facets"`
	}
	if err := json.Unmarshal(call(s.handleNotes, "GET", "/api/notes"+query, nil).Body.Bytes(), &out); err != nil {
		t.Fatalf("respuesta de /api/notes ilegible: %v", err)
	}
	return out.Notes, out.Total, out.Facets
}

// TestNotesIndice cubre el contrato del Vault como índice: filtros, búsqueda,
// orden, paginación y facetas. Es lo que sostiene que el vault siga siendo usable
// cuando tenga cientos de notas en vez de diez.
func TestNotesIndice(t *testing.T) {
	s := testServer(t)
	nota := func(id, proj, autor, cuerpo string) {
		d := map[string]any{"id": id, "type": "bug", "project": proj, "body": "## Claim\n" + cuerpo,
			"evidence": []map[string]string{{"kind": "file_read", "ref": "x.go:1"}}}
		call(s.handleCapture, "POST", "/api/capture", d)
		// el autor no viaja en el draft: se toma del caller, así que se fija a mano
		if n, err := core.ReadNoteFile(filepath.Join(s.dir, id+".md")); err == nil {
			n.Author = autor
			_ = core.WriteNoteFile(filepath.Join(s.dir, id+".md"), n)
		}
	}
	nota("a-redis", "tienda", "token:codex", "el cache de redis expira solo")
	nota("b-postgres", "tienda", "token:claude", "el indice de postgres esta mal")
	nota("c-redis", "cogo", "token:claude", "redis no se usa en cogo")

	// sin filtros: están las tres y las facetas las describen
	_, total, facets := notesOf(t, s, "")
	if total < 3 {
		t.Fatalf("total = %d, want >= 3", total)
	}
	if facets["projects"] == nil || facets["authors"] == nil || facets["colors"] == nil {
		t.Errorf("faltan facetas: %v", facets)
	}

	// filtro por proyecto
	if _, n, _ := notesOf(t, s, "?project=cogo"); n != 1 {
		t.Errorf("project=cogo -> %d, want 1", n)
	}
	// filtro por autor
	if _, n, _ := notesOf(t, s, "?author=token:codex"); n != 1 {
		t.Errorf("author=codex -> %d, want 1", n)
	}
	// búsqueda: trae las de redis (las dos propias + la sembrada, cuyo id ES
	// "redis") y NO la de postgres. Lo que importa no es el número sino que
	// filtre: un buscador que devuelve todo ordenado no es un buscador.
	notes, n, _ := notesOf(t, s, "?q=redis")
	if n != 3 {
		t.Errorf("q=redis -> %d, want 3 (a-redis, c-redis y la nota sembrada)", n)
	}
	for _, x := range notes {
		if x["id"] == "b-postgres" {
			t.Errorf("q=redis no debería traer b-postgres")
		}
	}
	// búsqueda + proyecto se combinan
	if _, n, _ := notesOf(t, s, "?q=redis&project=cogo"); n != 1 {
		t.Errorf("q=redis&project=cogo -> %d, want 1", n)
	}
	// paginación: el total NO cambia, lo que cambia es cuánto se trae
	pag, tot, _ := notesOf(t, s, "?limit=2")
	if len(pag) != 2 || tot < 3 {
		t.Errorf("limit=2 -> trajo %d de %d", len(pag), tot)
	}
	if resto, _, _ := notesOf(t, s, "?limit=2&offset=2"); len(resto) == 0 {
		t.Errorf("offset=2 debería traer el resto")
	}
	// un offset más allá del final no revienta: devuelve vacío
	if resto, _, _ := notesOf(t, s, "?offset=9999"); len(resto) != 0 {
		t.Errorf("offset fuera de rango debería devolver vacío, trajo %d", len(resto))
	}
	// las notas traen la fecha de verificación (lo que faltaba para poder triar)
	if len(notes) > 0 && notes[0]["verified"] == "" {
		t.Errorf("la nota debería exponer 'verified'")
	}
}

// TestNotesFechasYPaginado cubre lo que el visor necesita para no colapsar con
// miles de notas: el rango de fechas de creación, la faceta que lo acota, y el
// tamaño de página (incluido "todas").
func TestNotesFechasYPaginado(t *testing.T) {
	s := testServer(t)
	// La fecha de creación sale del historial, y el historial lo escribe un hook
	// global que instala el comando `serve` (cmd/cogo/serve.go). Sin él, el test
	// probaría un servidor que no existe en producción.
	core.SetWriteHook(func(path string, n *core.Note) {
		history.Record(filepath.Dir(path), n.ID, n.Confidence, n.ColorReason, core.Claim(n))
	})
	t.Cleanup(func() { core.SetWriteHook(nil) })

	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("p-%02d", i)
		call(s.handleCapture, "POST", "/api/capture", map[string]any{
			"id": id, "type": "bug", "project": "x", "body": "## Claim\nnota " + id,
			"evidence": []map[string]string{{"kind": "file_read", "ref": "x.go:1"}}})
	}
	notas, total, facets := notesOf(t, s, "?limit=all")
	if total < 7 || len(notas) != total {
		t.Fatalf("limit=all devolvió %d de %d", len(notas), total)
	}

	// La faceta de fechas tiene que existir y ser una fecha real: es lo que acota
	// el calendario del visor.
	fd, _ := facets["dates"].(map[string]any)
	if fd == nil || len(fd["min"].(string)) != 10 || len(fd["max"].(string)) != 10 {
		t.Fatalf("facets.dates = %v", facets["dates"])
	}
	hoy := fd["max"].(string)

	// Las 7 notas se acaban de crear: hoy las incluye a todas. La nota sembrada por
	// testServer se escribió ANTES del hook, así que no tiene historial ni fecha, y
	// queda FUERA de cualquier rango: "no sé cuándo se creó" no es "se creó acá".
	// Es el caso real de las notas anteriores a que COGO llevara historial.
	if _, n, _ := notesOf(t, s, "?from="+hoy+"&limit=all"); n != 7 {
		t.Errorf("from=hoy -> %d, want 7 (las creadas hoy; la sembrada no tiene fecha)", n)
	}
	sinFecha, _, _ := notesOf(t, s, "?from=2000-01-01&to="+hoy+"&limit=all")
	if len(sinFecha) != 7 {
		t.Errorf("un rango que abarca todo -> %d, want 7", len(sinFecha))
	}
	for _, x := range sinFecha {
		if x["id"] == "redis" {
			t.Errorf("una nota sin fecha de creación no debería entrar en un rango")
		}
	}
	if _, n, _ := notesOf(t, s, "?to=2000-01-01&limit=all"); n != 0 {
		t.Errorf("to=2000-01-01 -> %d, want 0", n)
	}
	if total != 8 {
		t.Errorf("total sin filtro = %d, want 8 (las 7 + la sembrada)", total)
	}
	if _, n, _ := notesOf(t, s, "?from=2999-01-01&limit=all"); n != 0 {
		t.Errorf("rango futuro -> %d, want 0", n)
	}

	// Paginado: el total NO cambia con el tamaño de página, solo la porción.
	pag1, total1, _ := notesOf(t, s, "?limit=3&offset=0")
	pag2, total2, _ := notesOf(t, s, "?limit=3&offset=3")
	if len(pag1) != 3 || len(pag2) != 3 || total1 != total || total2 != total {
		t.Fatalf("paginado: %d/%d y %d/%d", len(pag1), total1, len(pag2), total2)
	}
	if pag1[0]["id"] == pag2[0]["id"] {
		t.Errorf("dos páginas devolvieron la misma primera nota: %v", pag1[0]["id"])
	}
	// Y la creación viaja en cada nota, que es lo que la tarjeta muestra.
	if c, _ := pag1[0]["created"].(string); len(c) < 10 {
		t.Errorf("falta created en la nota: %v", pag1[0])
	}
}
