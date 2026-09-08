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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/auth"
)

// Lo que un token de SOLO LECTURA puede hacer se declara como lista de lo
// permitido, no de lo prohibido. Antes era al revés, y la lista de escritura se
// quedó corta tres veces: `gap`, `stash` y `lease` escribían y no estaban. Con
// una allowlist, un tool nuevo nace bloqueado hasta que alguien lo clasifique —
// y hay un test que exige que cada tool registrado esté en una de las dos
// listas.
var toolsDeLectura = map[string]bool{
	"pack": true, "recall": true, "search": true, "open": true,
	"xray": true, "guard": true, "reflect": true,
	// authorize no cambia el vault: pregunta. Deja una línea en el log, que es
	// auditoría, no escritura.
	"authorize": true,
}

// toolsDeEscritura existe para el test de clasificación: todo tool tiene que
// estar en una lista o en la otra.
var toolsDeEscritura = map[string]bool{
	"capture": true, "verify": true, "archive": true, "restore": true, "remove": true,
	"gap": true, "stash": true, "lease": true,
}

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
				if !mcpPermitidoSoloLectura(body) {
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

// mcpPermitidoSoloLectura decide sobre el cuerpo de una llamada JSON-RPC. Lo
// que no sea un tools/call (initialize, tools/list, ping, notificaciones) pasa;
// un tools/call pasa solo si el tool está en la lista de lectura.
func mcpPermitidoSoloLectura(body []byte) bool {
	if esBatch(body) {
		return false // la auditoría ya lo rechazó antes; acá es cinturón
	}
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &msg) != nil {
		return false // lo que no se entiende no se deja pasar
	}
	if msg.Method != "tools/call" {
		return true
	}
	return toolsDeLectura[msg.Params.Name]
}

// esBatch detecta un array JSON-RPC. La versión 2025-06-18 de MCP los eliminó
// del protocolo, y acá eran un agujero: el clasificador miraba un objeto, así
// que un array con un `capture` adentro pasaba el read-only y la auditoría sin
// que nadie lo viera.
func esBatch(body []byte) bool {
	b := bytes.TrimLeft(body, " \t\r\n")
	return len(b) > 0 && b[0] == '['
}

// rutasDeAdministracion son las superficies del dueño del vault: se entra con
// la raíz o con una sesión de persona, nunca con un token emitido. Un token
// con escritura sirve para escribir notas; no para fabricar más tokens, cambiar
// las claves del LLM o tocar los parámetros del motor.
var rutasDeAdministracion = []string{
	"/api/tokens", "/api/settings", "/api/parametros", "/api/audit", "/api/export",
	"/api/evidence-roots", "/api/github/map",
}

func esRutaDeAdministracion(path string) bool {
	for _, p := range rutasDeAdministracion {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// enforceAdmin corta las rutas de administración para todo lo que no sea
// administrador, con cualquier método: leer los settings también es leer las
// claves.
func enforceAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if esRutaDeAdministracion(r.URL.Path) && !auth.EsAdmin(r) {
			http.Error(w, "esta operación es de administración: requiere la raíz o una sesión de persona, no un token emitido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// blockedForReadOnly: para /api, un token de solo lectura puede hacer GET y
// los pocos POST que son cálculos puros. Todo lo demás, no.
func blockedForReadOnly(path, method string) bool {
	if esRutaDeAdministracion(path) {
		return true
	}
	if method == http.MethodGet {
		return false
	}
	switch path {
	case "/api/preview", "/api/guard", "/api/xray", "/api/pack":
		return false // analizan o previsualizan; no escriben
	}
	return true
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

// mcpBlanco saca de la llamada SOBRE QUÉ se llamó: la nota, el proyecto, el
// permiso. Es lo que convierte la auditoría de "alguien está vivo" en "alguien
// está trabajando acá", que es lo único accionable para el que llega después.
//
// Guarda identificadores, nunca texto libre: el `query` de un `search` o el
// cuerpo de un `capture` pueden decir cosas que no corresponde dejar en un log
// que se lee entero para armar un aviso. Un id no dice nada que el vault no
// diga ya.
func mcpBlanco(body []byte) (nota, proyecto string) {
	var msg struct {
		Params struct {
			Arguments struct {
				ID      string `json:"id"`
				Note    string `json:"note"`
				Name    string `json:"name"`
				Project string `json:"project"`
			} `json:"arguments"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &msg) != nil {
		return "", ""
	}
	a := msg.Params.Arguments
	for _, v := range []string{a.ID, a.Note, a.Name} {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), strings.TrimSpace(a.Project)
		}
	}
	return "", strings.TrimSpace(a.Project)
}

// --- audit log: who (which token/user) called which MCP tool, and when --------

type auditEntry struct {
	Time   string `json:"time"`
	Caller string `json:"caller"`
	Tool   string `json:"tool,omitempty"`
	Method string `json:"method"`
	Path   string `json:"path"`
	IP     string `json:"ip"`
	// Sobre qué se llamó. Es lo que lee internal/presencia para avisarle a un
	// agente que otro está en lo mismo — sin esto solo se sabría que hay alguien
	// conectado, que no alcanza para decidir nada.
	Nota     string `json:"nota,omitempty"`
	Proyecto string `json:"proyecto,omitempty"`
}

// defaultAuditMax caps the audit log so it can't grow without bound: the writer
// keeps only the most recent N entries (override with COGO_AUDIT_MAX; 0 = keep
// everything). Kept in sync with the display cap in internal/web (handleAudit).
const defaultAuditMax = 5000

// auditMax reads the entry cap from the environment (default defaultAuditMax).
// A value <= 0 disables trimming (unbounded, the old behavior).
func auditMax() int {
	if v := os.Getenv("COGO_AUDIT_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultAuditMax
}

// trimAudit keeps only the last max lines of the audit log, rewriting it
// atomically. A no-op when the file is already within budget or max <= 0.
func trimAudit(path string, max int) {
	if max <= 0 {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) <= max {
		return
	}
	keep := lines[len(lines)-max:]
	tmp := path + ".tmp"
	if os.WriteFile(tmp, []byte(strings.Join(keep, "\n")+"\n"), 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// auditMiddleware records MCP tool calls and API writes to .cogo/audit.jsonl. It
// runs after the auth gate, so auth.Caller(r) identifies who did it. The log is
// append-only but self-trimming: it's capped at auditMax() entries so it can't
// grow forever (trimmed at startup and periodically as it's written).
func auditMiddleware(dir string) func(http.Handler) http.Handler {
	var mu sync.Mutex
	path := filepath.Join(dir, ".cogo", "audit.jsonl")
	max := auditMax()
	trimAudit(path, max) // reclaim on boot in case the cap shrank
	sinceTrim := 0
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tool, nota, proyecto string
			record := false
			if r.URL.Path == "/mcp" && r.Method == http.MethodPost {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				_ = r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(body)) // restore for downstream
				if esBatch(body) {
					http.Error(w, "los batches JSON-RPC no están soportados: una llamada por request", http.StatusBadRequest)
					return
				}
				if tool = mcpToolName(body); tool != "" {
					record = true
					nota, proyecto = mcpBlanco(body)
				}
			} else if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet &&
				!strings.HasPrefix(r.URL.Path, "/api/audit") { // don't audit audit-management itself
				record = true
			}
			if record {
				caller := auth.Caller(r)
				if caller == "" {
					caller = "anon"
				}
				e := auditEntry{Time: time.Now().UTC().Format(time.RFC3339), Caller: caller, Tool: tool,
					Method: r.Method, Path: r.URL.Path, IP: clientIP(r), Nota: nota, Proyecto: proyecto}
				mu.Lock()
				if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
					b, _ := json.Marshal(e)
					_, _ = f.Write(append(b, '\n'))
					_ = f.Close()
				}
				if sinceTrim++; max > 0 && sinceTrim >= 200 {
					sinceTrim = 0
					trimAudit(path, max)
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

// proxiesConfiables son las redes desde las que se acepta X-Forwarded-For.
// Vacío = ninguna: la IP es la del socket, que detrás de un proxy es siempre la
// del proxy — y entonces el rate limit frena a todos juntos y la auditoría no
// identifica a nadie. Se declara con COGO_TRUSTED_PROXIES (CIDRs separados por
// coma); para un contenedor detrás del proxy del mismo host, las redes privadas.
var proxiesConfiables = leerCIDRs(os.Getenv("COGO_TRUSTED_PROXIES"))

func leerCIDRs(s string) []*net.IPNet {
	var out []*net.IPNet
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			if strings.Contains(p, ":") {
				p += "/128"
			} else {
				p += "/32"
			}
		}
		if _, n, err := net.ParseCIDR(p); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func clientIP(r *http.Request) string {
	return ipDelCliente(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), proxiesConfiables)
}

// ipDelCliente camina X-Forwarded-For de derecha a izquierda saltando los
// proxies confiables, y se queda con el primer salto que no lo es: ese es el
// cliente. Nunca se le cree al header si el socket no viene de un proxy
// declarado — el header lo escribe cualquiera.
func ipDelCliente(remote, xff string, confiables []*net.IPNet) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if !enRedes(host, confiables) {
		return host
	}
	saltos := strings.Split(xff, ",")
	for i := len(saltos) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(saltos[i])
		if ip == "" {
			continue
		}
		if !enRedes(ip, confiables) {
			return ip
		}
	}
	return host
}

func enRedes(ip string, redes []*net.IPNet) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	for _, n := range redes {
		if n.Contains(p) {
			return true
		}
	}
	return false
}
