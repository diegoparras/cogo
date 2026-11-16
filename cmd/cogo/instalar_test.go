package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex configura sus servidores MCP en TOML. COGO escribe su tabla entre dos
// marcas, así reinstalar la reemplaza y no toca nada de lo demás.
func TestTomlCodexStdioEsIdempotente(t *testing.T) {
	srv := servidorCodex{Command: `C:\cogo\cogo.exe`, Args: []string{"serve", "-vault", `E:\vault`}}
	una, err := tomlCodex(nil, "cogo", srv)
	if err != nil {
		t.Fatal(err)
	}
	dos, err := tomlCodex(una, "cogo", srv)
	if err != nil {
		t.Fatal(err)
	}
	if string(una) != string(dos) {
		t.Fatalf("no es idempotente:\n%s\n---\n%s", una, dos)
	}
	s := string(dos)
	for _, quiero := range []string{"[mcp_servers.cogo]", `command = 'C:\cogo\cogo.exe'`, `args = ['serve', '-vault', 'E:\vault']`} {
		if !strings.Contains(s, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, s)
		}
	}
}

// Lo que ya había queda, un cambio de URL reemplaza la tabla en vez de sumar
// otra, y el token nunca se escribe: Codex lo lee de la variable que se nombra.
func TestTomlCodexPreservaLoAjenoYNoEscribeElToken(t *testing.T) {
	previo := []byte("model = \"gpt-5\"\n\n[mcp_servers.otro]\ncommand = \"otro\"\n")
	una, err := tomlCodex(previo, "cogo", servidorCodex{URL: "https://viejo/mcp", TokenEnv: "COGO_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	dos, err := tomlCodex(una, "cogo", servidorCodex{URL: "https://nuevo/mcp", TokenEnv: "COGO_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(dos)
	if !strings.Contains(s, "model = \"gpt-5\"") || !strings.Contains(s, "[mcp_servers.otro]") {
		t.Errorf("se perdió lo que ya estaba:\n%s", s)
	}
	if strings.Count(s, "[mcp_servers.cogo]") != 1 || strings.Contains(s, "viejo") {
		t.Errorf("reinstalar tiene que reemplazar la tabla:\n%s", s)
	}
	for _, quiero := range []string{`url = 'https://nuevo/mcp'`, `bearer_token_env_var = 'COGO_TOKEN'`} {
		if !strings.Contains(s, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, s)
		}
	}
}

// Una tabla con el mismo nombre escrita a mano no se pisa: dos tablas iguales
// son un TOML inválido, y borrar la ajena sería peor.
func TestTomlCodexNoPisaUnaTablaAMano(t *testing.T) {
	previo := []byte("[mcp_servers.cogo]\ncommand = \"a mano\"\n")
	if _, err := tomlCodex(previo, "cogo", servidorCodex{Command: "cogo"}); err == nil {
		t.Fatal("tendría que negarse a duplicar [mcp_servers.cogo]")
	}
}

// `cogo install -agent cursor -hooks` deja .cursor/mcp.json y .cursor/hooks.json,
// y el token solo como referencia a COGO_TOKEN, que Cursor resuelve al cargar.
func TestInstallCursorEscribeMcpYHooksSinToken(t *testing.T) {
	raiz := t.TempDir()
	conf := filepath.Join(raiz, ".cursor", "mcp.json")
	err := cmdInstall([]string{"-agent", "cursor", "-http", "https://cogo.example/mcp", "-token", "secreto-cursor", "-o", conf, "-hooks", "-project", "tienda"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	h, err := os.ReadFile(filepath.Join(raiz, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, quiero := range []string{`"mcpServers"`, "https://cogo.example/mcp", "Bearer ${env:COGO_TOKEN}"} {
		if !strings.Contains(string(m), quiero) {
			t.Errorf("falta %s en mcp.json:\n%s", quiero, m)
		}
	}
	for _, quiero := range []string{"beforeShellExecution", "sessionStart", "-formato cursor", "-project tienda"} {
		if !strings.Contains(string(h), quiero) {
			t.Errorf("falta %s en hooks.json:\n%s", quiero, h)
		}
	}
	if strings.Contains(string(m)+string(h), "secreto-cursor") {
		t.Error("el token no puede quedar en los archivos de Cursor")
	}
}

// -sin-preguntar vuelve al bloqueo duro: para corridas sin nadie mirando.
func TestInstallSinPreguntar(t *testing.T) {
	raiz := t.TempDir()
	conf := filepath.Join(raiz, ".mcp.json")
	if err := cmdInstall([]string{"-vault", filepath.Join(raiz, "vault"), "-o", conf, "-hooks", "-sin-preguntar"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(raiz, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "-decide cogo") {
		t.Errorf("falta -decide cogo:\n%s", b)
	}
}

// `cogo install -agent codex -hooks` deja los dos archivos de Codex en .codex/.
func TestInstallCodexEscribeConfigYHooks(t *testing.T) {
	raiz := t.TempDir()
	vault := filepath.Join(raiz, "vault")
	conf := filepath.Join(raiz, ".codex", "config.toml")
	err := cmdInstall([]string{"-agent", "codex", "-vault", vault, "-o", conf, "-hooks", "-project", "tienda"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "[mcp_servers.cogo]") || !strings.Contains(string(b), "serve") {
		t.Errorf("config.toml:\n%s", b)
	}
	h, err := os.ReadFile(filepath.Join(raiz, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, quiero := range []string{"SessionStart", "PreToolUse", "Bash|apply_patch", "-project tienda"} {
		if !strings.Contains(string(h), quiero) {
			t.Errorf("falta %s en hooks.json:\n%s", quiero, h)
		}
	}
}
