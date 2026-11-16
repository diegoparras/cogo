package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// Un healthcheck que no puede fallar no es un healthcheck.
func TestHealthzFallaSiElVaultNoExiste(t *testing.T) {
	h := salud(filepath.Join(t.TempDir(), "no-existe"))
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin vault tiene que ser 503, dio %d", rec.Code)
	}
	h = salud(t.TempDir())
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("con vault tiene que ser 200, dio %d %s", rec.Code, rec.Body.String())
	}
}
