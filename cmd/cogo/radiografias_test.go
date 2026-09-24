package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func nombresDeTools(t *testing.T, srv *mcp.Server) map[string]string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, tool := range res.Tools {
		out[tool.Name] = tool.Description
	}
	return out
}

// Las radiografías se quedan en el binario y se presentan como lo que son:
// dos tools que no tocan el vault, y que `serve` puede dejar afuera.
func TestSinRadiografiasElAgenteVe14Tools(t *testing.T) {
	con := nombresDeTools(t, newMCPServer(t.TempDir()))
	if len(con) != 16 || con["guard"] == "" || con["xray"] == "" {
		t.Fatalf("por defecto son 16 tools con guard y xray: %d", len(con))
	}
	for _, n := range []string{"guard", "xray"} {
		if !strings.HasPrefix(con[n], "This does not read or write the vault") {
			t.Errorf("%s tiene que decir primero que no toca el vault: %q", n, con[n])
		}
	}
	sin := nombresDeTools(t, newMCPServerCon(t.TempDir(), opcionesMCP{SinRadiografias: true}))
	if len(sin) != 14 || sin["guard"] != "" || sin["xray"] != "" {
		t.Fatalf("sin radiografías son 14, sin guard ni xray: %d", len(sin))
	}
	for n := range con {
		if n != "guard" && n != "xray" && sin[n] == "" {
			t.Errorf("la bandera no puede sacar %q", n)
		}
	}
}
