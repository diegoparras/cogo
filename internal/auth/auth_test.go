package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// federated builds an enabled Auth without going through OIDC discovery — enough
// to exercise the gate and the session cookie offline (no Lockatus needed).
func federated() *Auth {
	return &Auth{enabled: true, federated: true, secret: []byte("test-secret"), flows: map[string]flow{}}
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

func serve(a *Auth, req *http.Request) int {
	rec := httptest.NewRecorder()
	a.Gate(ok).ServeHTTP(rec, req)
	return rec.Code
}

func TestSessionCookieRoundTrip(t *testing.T) {
	a := federated()
	w := httptest.NewRecorder()
	a.setSession(w, session{Email: "x@y.com", Name: "X", Exp: time.Now().Add(time.Hour).UnixMilli()})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	s := a.session(req)
	if s == nil || s.Email != "x@y.com" {
		t.Fatalf("session did not round-trip: %+v", s)
	}

	// A tampered cookie must be rejected.
	bad := httptest.NewRequest(http.MethodGet, "/", nil)
	bad.AddCookie(&http.Cookie{Name: cookieName, Value: "forged.signature"})
	if a.session(bad) != nil {
		t.Error("forged cookie accepted")
	}
}

func TestGate(t *testing.T) {
	// Standalone gates nothing.
	if code := serve(Disabled(), httptest.NewRequest(http.MethodGet, "/api/notes", nil)); code != http.StatusOK {
		t.Errorf("standalone should pass /api, got %d", code)
	}

	a := federated()
	// Federated + no session: /api and /mcp are blocked...
	if code := serve(a, httptest.NewRequest(http.MethodGet, "/api/notes", nil)); code != http.StatusUnauthorized {
		t.Errorf("federated unauth /api should 401, got %d", code)
	}
	if code := serve(a, httptest.NewRequest(http.MethodGet, "/mcp", nil)); code != http.StatusUnauthorized {
		t.Errorf("federated unauth /mcp should 401, got %d", code)
	}
	// ...but static assets and the login screen stay open.
	if code := serve(a, httptest.NewRequest(http.MethodGet, "/app.js", nil)); code != http.StatusOK {
		t.Errorf("static asset should stay open, got %d", code)
	}

	// Federated + valid session: /api passes.
	w := httptest.NewRecorder()
	a.setSession(w, session{Email: "x", Exp: time.Now().Add(time.Hour).UnixMilli()})
	req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	if code := serve(a, req); code != http.StatusOK {
		t.Errorf("authed /api should pass, got %d", code)
	}
}

func TestTokenGate(t *testing.T) {
	a := &Auth{enabled: true, token: "s3cret"}

	// No / wrong Bearer -> 401 on protected paths.
	if code := serve(a, httptest.NewRequest(http.MethodGet, "/mcp", nil)); code != http.StatusUnauthorized {
		t.Errorf("token mode, no bearer, /mcp should 401, got %d", code)
	}
	wrong := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	wrong.Header.Set("Authorization", "Bearer nope")
	if code := serve(a, wrong); code != http.StatusUnauthorized {
		t.Errorf("wrong bearer should 401, got %d", code)
	}

	// Correct Bearer -> passes.
	good := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	good.Header.Set("Authorization", "Bearer s3cret")
	if code := serve(a, good); code != http.StatusOK {
		t.Errorf("correct bearer should pass, got %d", code)
	}

	// Static assets stay open even in token mode.
	if code := serve(a, httptest.NewRequest(http.MethodGet, "/app.js", nil)); code != http.StatusOK {
		t.Errorf("static asset should stay open, got %d", code)
	}
}
