package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// El manual prometía que COGO se niega a guardar una nota con un secreto. El
// escáner solo corría sobre los artefactos; las notas —donde un agente pega el
// .env que acaba de leer— pasaban de largo.
func TestCaptureRechazaSecretos(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(dir).Run(ctx, serverT) }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "capture", Arguments: map[string]any{
		"type": "runbook", "project": "p", "id": "deploy-clave",
		"body": "## Claim\nPara deployar: STRIPE_SECRET_KEY=sk_live_ab12cd34ef56gh78ij90 y listo",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := toolText(res); !strings.Contains(txt, "Not stored") {
		t.Fatalf("tendría que negarse a guardar: %q", txt)
	}
	if _, err := os.Stat(filepath.Join(dir, "deploy-clave.md")); !os.IsNotExist(err) {
		t.Fatal("la nota con el secreto se escribió igual")
	}

	// Y lo mismo por el id: un `..` no produce un archivo afuera del vault.
	res, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "capture", Arguments: map[string]any{
		"type": "bug", "project": "p", "id": "../../pwn", "body": "## Claim\nx",
	}})
	if txt := toolText(res); !strings.Contains(txt, "..") {
		t.Fatalf("un id con .. tiene que rechazarse: %q", txt)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "..", "pwn.md")); !os.IsNotExist(err) {
		t.Fatal("se escribió fuera del vault")
	}
}
