package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Todo tool registrado tiene que estar clasificado como lectura o escritura.
// Es lo que hubiera atrapado a `gap`, `stash` y `lease`, que escribían y no
// estaban en la lista: con una allowlist un tool nuevo nace bloqueado, y este
// test obliga a decidir a sabiendas.
func TestTodaHerramientaEstaClasificada(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(t.TempDir()).Run(ctx, serverT) }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("el servidor no expone tools")
	}
	for _, tool := range res.Tools {
		l, e := toolsDeLectura[tool.Name], toolsDeEscritura[tool.Name]
		if l == e {
			t.Errorf("el tool %q tiene que estar en toolsDeLectura o en toolsDeEscritura (exactamente una): lectura=%v escritura=%v", tool.Name, l, e)
		}
	}
	for name := range toolsDeLectura {
		if toolsDeEscritura[name] {
			t.Errorf("%q está en las dos listas", name)
		}
	}
}

func TestSoloLecturaSobreMCP(t *testing.T) {
	llamada := func(tool string) string {
		return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":{}}}`
	}
	casos := []struct {
		nombre string
		body   string
		pasa   bool
	}{
		{"initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, true},
		{"tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, true},
		{"pack", llamada("pack"), true},
		{"authorize", llamada("authorize"), true},
		{"capture", llamada("capture"), false},
		{"gap escribe", llamada("gap"), false},
		{"stash escribe", llamada("stash"), false},
		{"lease escribe", llamada("lease"), false},
		{"tool desconocido nace bloqueado", llamada("nuevo-tool"), false},
		{"batch con un capture adentro", `[` + llamada("pack") + `,` + llamada("capture") + `]`, false},
		{"basura", `no es json`, false},
	}
	for _, c := range casos {
		if got := mcpPermitidoSoloLectura([]byte(c.body)); got != c.pasa {
			t.Errorf("%s: esperaba pasa=%v, dio %v", c.nombre, c.pasa, got)
		}
	}
}

func TestSoloLecturaSobreAPI(t *testing.T) {
	casos := []struct {
		metodo, path string
		bloqueado    bool
	}{
		{"GET", "/api/notes", false},
		{"GET", "/api/salaguerra", false},
		{"POST", "/api/preview", false},
		{"POST", "/api/capture", true},
		{"POST", "/api/leases", true},
		{"POST", "/api/agent-blocks", true},
		{"POST", "/api/ruta-que-no-existe-todavia", true}, // fail-closed
		{"GET", "/api/parametros", true},                  // administración: ni leer
		{"POST", "/api/parametros", true},
		{"GET", "/api/settings/models", true},
		{"GET", "/api/tokens", true},
	}
	for _, c := range casos {
		if got := blockedForReadOnly(c.path, c.metodo); got != c.bloqueado {
			t.Errorf("%s %s: esperaba bloqueado=%v, dio %v", c.metodo, c.path, c.bloqueado, got)
		}
	}
}

// Un token emitido —aunque tenga escritura— no administra: no fabrica tokens,
// no toca settings ni parámetros. Solo la raíz y una sesión de persona.
func TestAdministracionSoloParaLaRaiz(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := enforceAdmin(ok)
	casos := []struct {
		quien  string
		path   string
		status int
	}{
		{"token:agente-rw", "/api/tokens", http.StatusForbidden},
		{"token:agente-rw", "/api/settings/models", http.StatusForbidden},
		{"token:agente-rw", "/api/parametros", http.StatusForbidden},
		{"token:agente-rw", "/api/github/map", http.StatusForbidden},
		{"token:agente-rw", "/api/capture", http.StatusNoContent}, // escribir notas sí
		{"root", "/api/tokens", http.StatusNoContent},
		{"user:diego@x", "/api/tokens", http.StatusNoContent},
		{"", "/api/tokens", http.StatusNoContent}, // auth apagada (loopback)
	}
	for _, c := range casos {
		req := httptest.NewRequest("POST", c.path, nil)
		req = req.WithContext(auth.ConIdentidad(req.Context(), c.quien, false))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.status {
			t.Errorf("%q → %s: esperaba %d, dio %d", c.quien, c.path, c.status, rec.Code)
		}
	}
}

// Un batch JSON-RPC se rechaza para todos, antes de clasificar nada: era la
// forma de pasar un capture por debajo del read-only y de la auditoría.
func TestLosBatchesSeRechazan(t *testing.T) {
	llego := false
	h := auditMiddleware(t.TempDir())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { llego = true }))
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"capture"}}]`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || llego {
		t.Fatalf("el batch tendría que morir con 400 sin llegar al handler: code=%d llegó=%v", rec.Code, llego)
	}
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pack"}}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !llego {
		t.Fatal("una llamada normal tiene que pasar")
	}
}

// Detrás de un proxy, la IP del socket es siempre la del proxy: el rate limit
// frenaba a todos juntos y la auditoría no identificaba a nadie. Y al revés,
// creerle a X-Forwarded-For de cualquiera es dejar que el cliente elija su IP.
func TestIPDelClienteDetrasDelProxy(t *testing.T) {
	privadas := leerCIDRs("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")
	casos := []struct {
		nombre, remote, xff string
		confiables          []*net.IPNet
		quiero              string
	}{
		{"sin proxies declarados manda el socket", "203.0.113.5:4444", "1.2.3.4", nil, "203.0.113.5"},
		{"el header de un desconocido no vale", "203.0.113.5:4444", "1.2.3.4", privadas, "203.0.113.5"},
		{"detrás del proxy, el último salto no confiable", "172.18.0.2:4444", "1.2.3.4, 10.0.0.9", privadas, "1.2.3.4"},
		{"cadena con espacios y un solo salto", "10.1.1.1:80", " 198.51.100.7 ", privadas, "198.51.100.7"},
		{"si el header viene vacío, el socket", "10.1.1.1:80", "", privadas, "10.1.1.1"},
		{"un cliente no puede fabricarse un salto adicional adelante", "172.18.0.2:1", "9.9.9.9, 8.8.8.8", privadas, "8.8.8.8"},
		{"ipv6 sin puerto no rompe", "::1", "", privadas, "::1"},
	}
	for _, c := range casos {
		if got := ipDelCliente(c.remote, c.xff, c.confiables); got != c.quiero {
			t.Errorf("%s: esperaba %q, dio %q", c.nombre, c.quiero, got)
		}
	}
	if n := len(leerCIDRs("10.0.0.1, basura, 2001:db8::1")); n != 2 {
		t.Errorf("las IPs sueltas valen como /32 o /128 y la basura se ignora: %d", n)
	}
}
