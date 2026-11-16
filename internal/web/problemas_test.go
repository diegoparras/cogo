package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Un .md roto no puede dejar la pantalla en blanco: la lista sigue, y el aviso
// va arriba de todo, en la respuesta de la que se pinta el Vault.
func TestUnArchivoRotoSeAvisaEnLaLista(t *testing.T) {
	s := testServer(t)
	if err := os.WriteFile(filepath.Join(s.dir, "rota.md"), []byte("---\nid: rota\ntype: [no cierra\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/notes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("la lista tiene que responder igual: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Notes     []map[string]any `json:"notes"`
		Problemas []string         `json:"problemas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Notes) != 1 {
		t.Fatalf("la nota sana tiene que listarse: %d", len(out.Notes))
	}
	if len(out.Problemas) != 1 || !strings.Contains(out.Problemas[0], "rota.md") {
		t.Fatalf("el aviso tiene que nombrar el archivo: %v", out.Problemas)
	}
}
