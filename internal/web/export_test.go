package web

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// El backup lleva el estado —registro, historia, permisos, parámetros— y deja
// afuera los secretos y la caché. Antes excluía `.cogo` entero y la doc decía
// que ahí solo había caché.
func TestElExportLlevaElEstadoYNoLosSecretos(t *testing.T) {
	s := testServer(t)
	escribir := func(rel, contenido string) {
		p := filepath.Join(s.dir, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribir(".cogo/journal/2026.jsonl", `{"seq":1}`)
	escribir(".cogo/history/redis.jsonl", `{"time":"x"}`)
	escribir(".cogo/leases.json", `{}`)
	escribir(".cogo/parametros.json", `{}`)
	escribir(".cogo/tokens.json", `{"secreto":"cogo_xxx"}`)
	escribir(".cogo/llm.json", `{"api_key":"sk-xxx"}`)
	escribir(".cogo/audit.jsonl", `{"ip":"1.2.3.4"}`)
	escribir(".cogo/embeddings.bin", "binario")

	mux := http.NewServeMux()
	s.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/export", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	hay := map[string]bool{}
	for _, f := range zr.File {
		hay[f.Name] = true
	}
	for _, quiero := range []string{"redis.md", ".cogo/journal/2026.jsonl", ".cogo/history/redis.jsonl", ".cogo/leases.json", ".cogo/parametros.json"} {
		if !hay[quiero] {
			t.Errorf("falta %s en el backup", quiero)
		}
	}
	for _, noQuiero := range []string{".cogo/tokens.json", ".cogo/llm.json", ".cogo/audit.jsonl", ".cogo/embeddings.bin"} {
		if hay[noQuiero] {
			t.Errorf("%s no puede ir en el backup", noQuiero)
		}
	}
}
