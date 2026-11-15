package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Un COGO hosteado no necesita el binario en la máquina del agente: Claude
// Code y Codex pueden llamar a la tool desde el hook. authorize, llamado así,
// decide lo mismo que `cogo hook pre-tool` y contesta en el JSON del hook.
func TestAuthorizeEnModoHook(t *testing.T) {
	soltarRegistro()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(t.TempDir()).Run(ctx, serverT) }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	llamar := func(args map[string]any) (decision, motivo string) {
		t.Helper()
		// Como lo manda el hook: `action` es obligatorio en el esquema, y va con
		// el mismo placeholder; COGO decide desde los campos hook_*.
		args["action"] = "${tool_input.command}"
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "authorize", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("authorize en modo hook no puede dar error: %s", toolText(res))
		}
		var r map[string]any
		if err := json.Unmarshal([]byte(toolText(res)), &r); err != nil {
			t.Fatalf("el hook necesita JSON, dio %q", toolText(res))
		}
		h, _ := r["hookSpecificOutput"].(map[string]any)
		if h != nil && h["hookEventName"] != "PreToolUse" {
			t.Errorf("hookEventName: %v", h)
		}
		d, _ := h["permissionDecision"].(string)
		m, _ := h["permissionDecisionReason"].(string)
		return d, m
	}

	if d, m := llamar(map[string]any{"hook": "claude-code", "hook_tool": "Bash", "hook_command": "rm -rf build"}); d != "deny" || !strings.HasPrefix(m, "NOT AUTHORIZED") {
		t.Errorf("rm -rf sin respaldo se niega con el veredicto: %q %q", d, m)
	}
	if d, _ := llamar(map[string]any{"hook": "claude-code", "hook_tool": "Bash", "hook_command": "ls -la"}); d != "" {
		t.Errorf("ls no tiene objeción: %q", d)
	}
	parche := "*** Begin Patch\n*** Delete File: migraciones/001.sql\n*** End Patch\n"
	if d, _ := llamar(map[string]any{"hook": "codex", "hook_tool": "apply_patch", "hook_command": parche}); d != "deny" {
		t.Errorf("un parche de Codex que borra se niega: %q", d)
	}
	if d, _ := llamar(map[string]any{"hook": "claude-code", "hook_tool": "Edit", "hook_path": "/p/a.go", "hook_minimum": "reversible"}); d != "deny" {
		t.Errorf("con mínima reversible, editar sin respaldo se niega: %q", d)
	}
	// Si el harness no resolvió el placeholder, no hay acción que juzgar.
	if d, _ := llamar(map[string]any{"hook": "claude-code", "hook_tool": "Bash", "hook_command": "${tool_input.command}"}); d != "" {
		t.Errorf("un placeholder sin resolver no es una acción: %q", d)
	}
}

// `cogo install -via-mcp` deja hooks que llaman a la tool, no al binario.
// Cursor no tiene hooks de ese tipo, así que se niega.
func TestInstallViaMCP(t *testing.T) {
	raiz := t.TempDir()
	conf := filepath.Join(raiz, ".codex", "config.toml")
	if err := cmdInstall([]string{"-agent", "codex", "-http", "https://cogo.example/mcp", "-o", conf, "-hooks", "-via-mcp", "-project", "tienda"}); err != nil {
		t.Fatal(err)
	}
	h, err := os.ReadFile(filepath.Join(raiz, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(h)
	if !strings.Contains(s, `"type": "mcp_tool"`) || strings.Contains(s, " hook pre-tool") {
		t.Errorf("los hooks tienen que llamar a la tool, no al binario:\n%s", s)
	}
	if err := cmdInstall([]string{"-agent", "cursor", "-http", "https://cogo.example/mcp", "-o", filepath.Join(raiz, ".cursor", "mcp.json"), "-hooks", "-via-mcp"}); err == nil {
		t.Error("Cursor no tiene hooks mcp_tool: -via-mcp tiene que fallar")
	}
	if err := cmdInstall([]string{"-agent", "codex", "-o", conf, "-hooks", "-via-mcp"}); err == nil {
		t.Error("-via-mcp sin -http no tiene servidor al que llamar")
	}
}
