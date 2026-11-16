package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegoparras/cogo/internal/auth"
)

// Dos tokens desde la misma IP tienen cupos separados: el que se pasa es el
// único que se frena. Y una petición sin identidad no toca este limitador —
// para eso está el de IP, afuera del gate.
func TestElCupoPorTokenEsPorToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := newTokenLimiter(0, 2).middleware(ok) // sin recarga: dos y basta

	pedir := func(quien string) int {
		req := httptest.NewRequest("POST", "/mcp", nil)
		req.RemoteAddr = "10.0.0.7:1234" // misma IP para todos
		if quien != "" {
			req = req.WithContext(auth.ConIdentidad(req.Context(), quien, false))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if pedir("token:a") != 204 || pedir("token:a") != 204 {
		t.Fatal("las dos primeras de a pasan")
	}
	if pedir("token:a") != http.StatusTooManyRequests {
		t.Fatal("la tercera de a se frena")
	}
	if pedir("token:b") != 204 {
		t.Fatal("b tiene su propio cupo, aunque sea la misma IP")
	}
	for i := 0; i < 5; i++ {
		if pedir("") != 204 {
			t.Fatal("sin identidad este limitador no aplica")
		}
	}
	// Fuera de /api y /mcp no cuenta.
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(auth.ConIdentidad(req.Context(), "token:a", false))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatal("la raíz no se limita por token")
	}
}
