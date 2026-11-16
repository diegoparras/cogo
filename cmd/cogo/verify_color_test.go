package main

import (
	"context"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/history"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sesionVerify conecta un cliente MCP sin identidad, como stdio o un COGO local
// sin autenticación, y deja capturada una nota amarilla lista para verificar.
func sesionVerify(t *testing.T, dir string) func(tool string, args map[string]any) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(dir).Run(ctx, serverT) }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })

	llamar := func(tool string, args map[string]any) string {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return res.Content[0].(*mcp.TextContent).Text
	}
	capturada := llamar("capture", map[string]any{
		"type": "runbook", "project": "p", "id": "build-compila",
		"body":       "## Claim\n`go build ./...` compila sin errores.",
		"check_test": "go build ./...",
		"evidence":   []map[string]any{{"kind": "command_output", "ref": "go build ./... → ok (hoy)"}},
	})
	if !strings.Contains(capturada, "as yellow") {
		t.Fatalf("precondición: sin verificar tenía que entrar amarilla: %s", capturada)
	}
	return llamar
}

// Sin nadie identificado, verify declara igual, a nombre de "sin identificar".
// Antes no hacía nada: decidía si declarar comparando contra una Verificacion
// vacía, y sin identidad ni reanchor la Verificacion ES vacía. La nota seguía
// amarilla para siempre y la respuesta parecía la de una verificación. Es el
// camino de stdio, el del .mcpb de Claude Desktop.
func TestVerifySinIdentidadDeclaraIgual(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	dir := t.TempDir()
	conVault(&dir)
	llamar := sesionVerify(t, dir)

	dijo := llamar("verify", map[string]any{"id": "build-compila"})

	vault, err := core.LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := vault["build-compila"].Check
	if c.Status != "passed" || c.Attestation() != core.AttestDeclared || c.AttestedBy != "sin identificar" {
		t.Fatalf("verify sin identidad tenía que declarar el check: status=%q attested=%q por=%q; contestó %q",
			c.Status, c.Attestation(), c.AttestedBy, dijo)
	}
}

// verify contesta con el color que la nota tiene DESPUÉS de verificarse.
//
// Con el motor de eventos el color se pliega desde el registro, y la
// verificación entra al registro recién cuando la nota se escribe. Si el color
// se pide antes de escribir, el motor contesta por la nota de antes: el agente
// lee amarillo sobre una nota que ya es verde, y el archivo y el historial se
// quedan con ese amarillo. Pasó en producción el 2026-11-16.
func TestVerifyDevuelveElColorDeDespues(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	dir := t.TempDir()
	conVault(&dir)
	if err := instalarMotor(dir); err != nil {
		t.Fatal(err)
	}
	llamar := sesionVerify(t, dir)

	dijo := llamar("verify", map[string]any{"id": "build-compila"})

	vault, err := core.LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	ahora := core.Evaluate(vault["build-compila"], vault, nil, today())
	if ahora.Color != core.Green {
		t.Fatalf("precondición: verificada, la nota tiene que estar verde; está %s (%s)", ahora.Color, ahora.Reason)
	}
	if !strings.HasPrefix(dijo, "green ") {
		t.Errorf("verify contestó %q, y la nota ya está %s", dijo, ahora.Color)
	}
	if got := vault["build-compila"].Confidence; got != "green" {
		t.Errorf("el archivo quedó con confidence %q después de verificar", got)
	}
	versiones := history.Load(dir, "build-compila")
	if len(versiones) == 0 || versiones[len(versiones)-1].Color != "green" {
		t.Errorf("la última versión del historial tiene que ser la verde: %+v", versiones)
	}
}
