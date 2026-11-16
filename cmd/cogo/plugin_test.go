package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/agentsmd"
	"github.com/diegoparras/cogo/internal/gancho"
	"github.com/diegoparras/cogo/internal/web"
)

// El plugin de Claude Code vive en plugins/cogo, y el repo entero es su
// marketplace. El skill y los hooks salen del código —el protocolo de
// agentsmd y los hooks MCP de gancho—, así que no se escriben a mano: este
// test los compara con lo que generaría COGO hoy. Si cambia el protocolo o
// los hooks, se regeneran con:
//
//	COGO_ACTUALIZAR_PLUGIN=1 go test ./cmd/cogo -run TestElPluginEstaAlDia
func TestElPluginEstaAlDia(t *testing.T) {
	raiz := filepath.Join("..", "..")
	plugin := filepath.Join(raiz, "plugins", "cogo")
	hooks, err := gancho.SettingsMCP(nil, gancho.ViaMCP{Servidor: "plugin:cogo:cogo", Minima: "costly"})
	if err != nil {
		t.Fatal(err)
	}
	generados := map[string][]byte{
		filepath.Join(plugin, "skills", "cogo", "SKILL.md"): []byte(agentsmd.Skill()),
		filepath.Join(plugin, "hooks", "hooks.json"):        hooks,
	}
	actualizar := os.Getenv("COGO_ACTUALIZAR_PLUGIN") == "1"
	for ruta, quiero := range generados {
		if actualizar {
			if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(ruta, quiero, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatalf("falta %s: %v", ruta, err)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != string(quiero) {
			t.Errorf("%s quedó viejo: regeneralo con COGO_ACTUALIZAR_PLUGIN=1 go test ./cmd/cogo -run TestElPluginEstaAlDia", ruta)
		}
	}

	leer := func(ruta string, v any) {
		t.Helper()
		b, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
	}
	// El manifiesto del plugin: se llama como el servidor que nombran los
	// hooks (plugin:cogo:cogo), y su versión es la del binario.
	var pj struct {
		Name       string                    `json:"name"`
		Version    string                    `json:"version"`
		UserConfig map[string]map[string]any `json:"userConfig"`
	}
	leer(filepath.Join(plugin, ".claude-plugin", "plugin.json"), &pj)
	if pj.Name != "cogo" || pj.Version != version {
		t.Errorf("plugin.json: nombre %q, versión %q (el binario es %s)", pj.Name, pj.Version, version)
	}
	// Y el visor muestra la misma: una sola versión, en los tres lugares.
	if web.Version != version {
		t.Errorf("el visor dice %s y el binario %s: se suben juntas", web.Version, version)
	}
	if pj.UserConfig["token"]["sensitive"] != true || pj.UserConfig["url"]["required"] != true {
		t.Errorf("el token va enmascarado y la dirección es obligatoria: %v", pj.UserConfig)
	}
	// El servidor MCP usa lo que la persona cargó al habilitar el plugin.
	var mc struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	leer(filepath.Join(plugin, ".mcp.json"), &mc)
	srv, _ := json.Marshal(mc.MCPServers["cogo"])
	if !strings.Contains(string(srv), "${user_config.url}") || !strings.Contains(string(srv), "Bearer ${user_config.token}") {
		t.Errorf(".mcp.json: %s", srv)
	}
	// Y el marketplace, en la raíz del repo, apunta al plugin.
	var mk struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	leer(filepath.Join(raiz, ".claude-plugin", "marketplace.json"), &mk)
	if len(mk.Plugins) != 1 || mk.Plugins[0].Name != "cogo" || mk.Plugins[0].Source != "./plugins/cogo" {
		t.Errorf("marketplace.json: %+v", mk)
	}
}
