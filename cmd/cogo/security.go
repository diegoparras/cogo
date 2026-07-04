package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/auth"
)

// enforceReadOnly refuses write operations for requests authorized by a
// read-only token. It runs AFTER the auth gate (which stamps the scope on the
// request context). For /api it classifies by path+method; for /mcp it peeks the
// JSON-RPC body to see which tool is being called.
func enforceReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.ReadOnlyGranted(r) {
			if r.URL.Path == "/mcp" && r.Method == http.MethodPost {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				_ = r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(body)) // restore for the real handler
				if isWriteMCPCall(body) {
					forbidReadOnly(w)
					return
				}
			} else if blockedForReadOnly(r.URL.Path, r.Method) {
				forbidReadOnly(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func forbidReadOnly(w http.ResponseWriter) {
	http.Error(w, "este token es de solo lectura: la operación requiere un token con permiso de escritura", http.StatusForbidden)
}

func blockedForReadOnly(path, method string) bool {
	if path == "/api/tokens" || path == "/api/settings" || path == "/api/audit" {
		return true // tokens, settings and the audit trail are admin, never a read-only agent
	}
	if method == http.MethodGet {
		return false // reads are always allowed
	}
	switch path {
	case "/api/capture", "/api/verify", "/api/archive", "/api/restore", "/api/delete", "/api/mandate", "/api/lint", "/api/contradictions", "/api/trash", "/api/guard/label":
		return true
	}
	return false
}

// mcpToolName pulls params.name from a JSON-RPC tools/call body ("" otherwise).
func mcpToolName(body []byte) string {
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &msg) != nil || msg.Method != "tools/call" {
		return ""
	}
	return msg.Params.Name
}

func isWriteMCPCall(body []byte) bool {
	switch mcpToolName(body) {
	case "capture", "verify", "archive", "restore", "remove":
		return true
	}
	return false
}

// --- audit log: who (which token/user) called which MCP tool, and when --------

type auditEntry struct {
	Time   string `json:"time"`
	Caller string `json:"caller"`
	Tool   string `json:"tool,omitempty"`
	Method string `json:"method"`
	Path   string `json:"path"`
	IP     string `json:"ip"`
}

// auditMiddleware records MCP tool calls and API writes to .cogo/audit.jsonl. It
// runs after the auth gate, so auth.Caller(r) identifies who did it.
func auditMiddleware(dir string) func(http.Handler) http.Handler {
	var mu sync.Mutex
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tool string
			record := false
			if r.URL.Path == "/mcp" && r.Method == http.MethodPost {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				_ = r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(body)) // restore for downstream
				if tool = mcpToolName(body); tool != "" {
					record = true
				}
			} else if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet {
				record = true
			}
			if record {
				caller := auth.Caller(r)
				if caller == "" {
					caller = "anon"
				}
				e := auditEntry{Time: time.Now().UTC().Format(time.RFC3339), Caller: caller, Tool: tool, Method: r.Method, Path: r.URL.Path, IP: clientIP(r)}
				mu.Lock()
				if f, err := os.OpenFile(filepath.Join(dir, ".cogo", "audit.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
					b, _ := json.Marshal(e)
					_, _ = f.Write(append(b, '\n'))
					_ = f.Close()
				}
				mu.Unlock()
			}
			next.ServeHTTP(w, r)
		})
	}
}

// checkExposure is the fail-safe: refuse to serve on a public interface with no
// authentication, so an unauthenticated vault + MCP can't land on the open
// internet by accident. An operator who knows the port is already private
// (firewall, SSH tunnel, VPN) overrides with COGO_ALLOW_INSECURE=1.
func checkExposure(addr string, a *auth.Auth) error {
	if a.Enabled() || isLoopback(addr) || os.Getenv("COGO_ALLOW_INSECURE") == "1" {
		return nil
	}
	return fmt.Errorf("refusing to serve on a non-loopback address (%s) with NO authentication.\n"+
		"  Anyone who can reach this port could read/write your vault and drive the MCP.\n"+
		"  Fix one of:\n"+
		"    - COGO_MCP_TOKEN=<secret>          Bearer auth for the MCP/API (recommended for a VPS)\n"+
		"    - AUTH_MODE=federado + LOCKATUS_*  OIDC login (Lockatus)\n"+
		"    - bind 127.0.0.1 and reach it over an SSH tunnel or VPN\n"+
		"  Or, if this port is already firewalled/private, set COGO_ALLOW_INSECURE=1 to override", addr)
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false // e.g. a bare ":8080" -> all interfaces -> not loopback
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// securityHeaders sets conservative defaults on every response. HSTS only when
// behind TLS (COOKIE_SECURE=1), so it never traps a plain-HTTP loopback dev run.
func securityHeaders(next http.Handler, tls bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if tls {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// --- per-IP rate limit (token bucket) on the sensitive paths ----------------
// Caps brute-force against the token and abuse of the model-spending endpoints
// (Guard/lint). Generous enough for a human plus one agent.

type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64 // tokens per second
	burst   float64
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newIPLimiter(rate, burst float64) *ipLimiter {
	return &ipLimiter{buckets: map[string]*tokenBucket{}, rate: rate, burst: burst}
}

func (l *ipLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[ip]
	if b == nil {
		l.buckets[ip] = &tokenBucket{tokens: l.burst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *ipLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" {
			if !l.allow(clientIP(r), time.Now()) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
