package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// La spec de MCP del 2026-07-28 saca el initialize y la sesión: cada pedido
// viaja solo, con su versión adentro. El servidor HTTP de COGO tiene que
// hablarla sin dejar afuera a los clientes que siguen en la 2025-11-25.
func TestMCPHTTPHablaLaSpecSinEstado(t *testing.T) {
	ts := httptest.NewServer(handlerMCP(newMCPServer(t.TempDir())))
	defer ts.Close()

	casos := []struct {
		nombre, pide, negocia string
	}{
		{"cliente nuevo", "", "2026-07-28"},
		{"cliente de la spec anterior", "2025-11-25", "2025-11-25"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cli := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			cs, err := cli.Connect(ctx,
				&mcp.StreamableClientTransport{Endpoint: ts.URL, DisableStandaloneSSE: true},
				&mcp.ClientSessionOptions{ProtocolVersion: c.pide})
			if err != nil {
				t.Fatalf("no conecta: %v", err)
			}
			defer cs.Close()

			if got := cs.InitializeResult().ProtocolVersion; got != c.negocia {
				t.Fatalf("negoció %q, quería %q", got, c.negocia)
			}
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "pack", Arguments: map[string]any{"query": ""}})
			if err != nil {
				t.Fatalf("pack: %v", err)
			}
			if res.IsError {
				t.Fatalf("pack devolvió error: %s", toolText(res))
			}
		})
	}
}

// Sin estado, un redeploy no deja colgado a nadie: un pedido con un
// Mcp-Session-Id que el servidor nunca emitió se atiende igual.
func TestMCPHTTPNoDependeDeLaSesion(t *testing.T) {
	ts := httptest.NewServer(handlerMCP(newMCPServer(t.TempDir())))
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req := httptest.NewRequest("POST", ts.URL, strings.NewReader(body))
	req.RequestURI = ""
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	req.Header.Set("Mcp-Session-Id", "una-sesion-de-antes-del-redeploy")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("una sesión desconocida no debería importar; status %d", resp.StatusCode)
	}
}
