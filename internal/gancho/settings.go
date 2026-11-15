package gancho

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Lo que se escribe en .claude/settings.local.json.
//
// Va en el `.local` y no en `settings.json` porque lleva la ruta absoluta del
// binario —que es de esta máquina— y, en modo remoto, el token. Claude Code
// carga los dos; el local no se commitea.

// Comando es cómo invocar a cogo desde el hook: el binario y los flags que
// comparten todos los subcomandos (vault, o url+token, y proyecto).
type Comando struct {
	Binario  string
	Vault    string // modo local
	URL      string // modo remoto; si está, gana sobre Vault
	Token    string
	Proyecto string
	Minima   string // clase mínima que dispara la consulta
	Formato  string // cómo contestarle al harness: vacío para Claude Code y Codex, "cursor"
}

// marcaCogo es lo que identifica a nuestros hooks dentro del archivo, para
// reemplazarlos sin tocar los ajenos.
const marcaCogo = " hook "

func (c Comando) linea(sub string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s hook %s", comillas(c.Binario), sub)
	if c.URL != "" {
		fmt.Fprintf(&b, " -url %s", comillas(c.URL))
		if c.Token != "" {
			fmt.Fprintf(&b, " -token %s", comillas(c.Token))
		}
	} else if c.Vault != "" {
		fmt.Fprintf(&b, " -vault %s", comillas(c.Vault))
	}
	if c.Proyecto != "" {
		fmt.Fprintf(&b, " -project %s", comillas(c.Proyecto))
	}
	if c.Minima != "" && sub == "pre-tool" {
		fmt.Fprintf(&b, " -minima %s", c.Minima)
	}
	if c.Formato != "" {
		fmt.Fprintf(&b, " -formato %s", c.Formato)
	}
	return b.String()
}

func comillas(s string) string {
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// Settings mezcla los hooks de COGO en un settings.json existente (o vacío),
// preservando todo lo demás: otros hooks, permisos, lo que haya. Los hooks de
// COGO previos se reemplazan, así correr install dos veces no los duplica.
func Settings(existente []byte, c Comando) ([]byte, error) {
	return mezclar(existente, c, "Bash|Edit|Write|MultiEdit|NotebookEdit")
}

// HooksCodex hace lo mismo para el .codex/hooks.json de Codex, que tiene la
// misma forma que el de Claude Code. Dos diferencias: las ediciones llegan como
// `apply_patch`, y el archivo vive en el repo sin variante local, así que el
// token no se escribe nunca: el hook lo lee de COGO_TOKEN.
func HooksCodex(existente []byte, c Comando) ([]byte, error) {
	c.Token = ""
	return mezclar(existente, c, "Bash|apply_patch")
}

// HooksCursor mezcla los hooks de COGO en el .cursor/hooks.json de Cursor, que
// tiene otra forma: un `version` y, por evento, una lista de comandos sueltos.
// Los comandos de shell llegan por beforeShellExecution y las ediciones y los
// borrados por preToolUse, como Write y Delete. El hook contesta en el formato
// de Cursor, y el token tampoco se escribe: se lee de COGO_TOKEN.
func HooksCursor(existente []byte, c Comando) ([]byte, error) {
	c.Token = ""
	c.Formato = "cursor"
	root := map[string]any{}
	if strings.TrimSpace(string(existente)) != "" {
		if err := json.Unmarshal(existente, &root); err != nil {
			return nil, fmt.Errorf("hooks.json existente: %w", err)
		}
	}
	if _, ok := root["version"]; !ok {
		root["version"] = 1
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	poner := func(evento, matcher, sub string, timeout int) {
		lista, _ := hooks[evento].([]any)
		var limpia []any
		for _, it := range lista {
			m, _ := it.(map[string]any)
			if !esComandoCogo(m["command"]) {
				limpia = append(limpia, it)
			}
		}
		entrada := map[string]any{"command": c.linea(sub), "timeout": timeout}
		if matcher != "" {
			entrada["matcher"] = matcher
		}
		hooks[evento] = append(limpia, entrada)
	}
	poner("sessionStart", "", "session-start", 30)
	poner("beforeShellExecution", "", "pre-tool", 20)
	poner("preToolUse", "Write|Delete", "pre-tool", 20)

	root["hooks"] = hooks
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// ViaMCP son los hooks que llaman a las tools del servidor MCP en vez de al
// binario: para un COGO hosteado, sin nada instalado en la máquina del agente.
// Claude Code y Codex los soportan; Cursor no.
//
// Una diferencia con los de comando: al arrancar la sesión, los hooks mcp_tool
// se saltean porque los servidores todavía no están conectados. Por eso el de
// SessionStart corre en compact, resume y clear, y re-ancla con `recall`; el
// pack del arranque lo pide el agente, como dice su AGENTS.md.
type ViaMCP struct {
	Servidor string // el nombre del servidor MCP en la config del harness
	Proyecto string
	Minima   string
}

const matcherReanclar = "compact|resume|clear"

// authorize arma la llamada. `action` es obligatorio en el esquema de la tool,
// que es el contrato que ven los agentes; el hook lo manda con el mismo texto
// que juzga, aunque en modo hook COGO decide desde los campos hook_*.
func (v ViaMCP) authorize(harness, tool, action string, campos map[string]any) map[string]any {
	input := map[string]any{"hook": harness, "hook_tool": tool, "action": action}
	for k, val := range campos {
		input[k] = val
	}
	if v.Proyecto != "" {
		input["project"] = v.Proyecto
	}
	if v.Minima != "" {
		input["hook_minimum"] = v.Minima
	}
	return map[string]any{"type": "mcp_tool", "server": v.Servidor, "tool": "authorize", "input": input, "timeout": 20}
}

func (v ViaMCP) recall() map[string]any {
	input := map[string]any{}
	if v.Proyecto != "" {
		input["project"] = v.Proyecto
	}
	return map[string]any{"type": "mcp_tool", "server": v.Servidor, "tool": "recall", "input": input, "timeout": 30}
}

// SettingsMCP son los hooks de Claude Code en modo MCP. Los campos del harness
// van sueltos y como texto (${tool_input.command}, ${tool_input.file_path}),
// que es lo que Claude Code documenta que reemplaza.
func SettingsMCP(existente []byte, v ViaMCP) ([]byte, error) {
	return mezclarEntradas(existente, map[string][]any{
		"SessionStart": {entrada(matcherReanclar, v.recall())},
		"PreToolUse": {
			entrada("Bash", v.authorize("claude-code", "Bash", "${tool_input.command}",
				map[string]any{"hook_command": "${tool_input.command}"})),
			entrada("Edit|Write|MultiEdit", v.authorize("claude-code", "${tool_name}", "${tool_name} ${tool_input.file_path}",
				map[string]any{"hook_path": "${tool_input.file_path}"})),
			entrada("NotebookEdit", v.authorize("claude-code", "NotebookEdit", "NotebookEdit ${tool_input.notebook_path}",
				map[string]any{"hook_path": "${tool_input.notebook_path}"})),
		},
	})
}

// HooksCodexMCP son los de Codex en modo MCP: Bash y apply_patch traen los dos
// lo que hay que juzgar en tool_input.command.
func HooksCodexMCP(existente []byte, v ViaMCP) ([]byte, error) {
	return mezclarEntradas(existente, map[string][]any{
		"SessionStart": {entrada(matcherReanclar, v.recall())},
		"PreToolUse": {
			entrada("Bash|apply_patch", v.authorize("codex", "${tool_name}", "${tool_input.command}",
				map[string]any{"hook_command": "${tool_input.command}"})),
		},
	})
}

func mezclar(existente []byte, c Comando, matcherPre string) ([]byte, error) {
	comando := func(sub string, timeout int) map[string]any {
		return map[string]any{"type": "command", "command": c.linea(sub), "timeout": timeout}
	}
	return mezclarEntradas(existente, map[string][]any{
		// El pack al empezar: entra como contexto sin que nadie lo pida.
		"SessionStart": {entrada("", comando("session-start", 30))},
		// Y la pregunta antes de tocar algo. Solo las herramientas que cambian
		// cosas afuera de la conversación.
		"PreToolUse": {entrada(matcherPre, comando("pre-tool", 20))},
	})
}

func entrada(matcher string, handler map[string]any) any {
	e := map[string]any{"hooks": []any{handler}}
	if matcher != "" {
		e["matcher"] = matcher
	}
	return e
}

// eventosCogo son los eventos donde COGO escribe hooks, en cualquier modo. Al
// mezclar se limpian todos, así pasar de comando a MCP no deja restos.
var eventosCogo = []string{"SessionStart", "PreToolUse"}

func mezclarEntradas(existente []byte, nuestras map[string][]any) ([]byte, error) {
	root := map[string]any{}
	if strings.TrimSpace(string(existente)) != "" {
		if err := json.Unmarshal(existente, &root); err != nil {
			return nil, fmt.Errorf("settings existente: %w", err)
		}
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, evento := range eventosCogo {
		lista, _ := hooks[evento].([]any)
		var limpia []any
		for _, it := range lista {
			if !esNuestro(it) {
				limpia = append(limpia, it)
			}
		}
		limpia = append(limpia, nuestras[evento]...)
		if len(limpia) > 0 {
			hooks[evento] = limpia
		} else {
			delete(hooks, evento)
		}
	}
	root["hooks"] = hooks
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// esNuestro reconoce una entrada de hook escrita por COGO: alguno de sus
// comandos invoca `cogo hook`, o llama a authorize en modo hook, o re-ancla
// con recall después de compactar.
func esNuestro(it any) bool {
	m, ok := it.(map[string]any)
	if !ok {
		return false
	}
	lista, _ := m["hooks"].([]any)
	for _, h := range lista {
		hm, _ := h.(map[string]any)
		if esComandoCogo(hm["command"]) {
			return true
		}
		if hm["type"] != "mcp_tool" {
			continue
		}
		input, _ := hm["input"].(map[string]any)
		if _, conHook := input["hook"]; hm["tool"] == "authorize" && conHook {
			return true
		}
		if hm["tool"] == "recall" && m["matcher"] == matcherReanclar {
			return true
		}
	}
	return false
}

// esComandoCogo reconoce un comando de hook que invoca `cogo hook`.
func esComandoCogo(v any) bool {
	cmd, _ := v.(string)
	return strings.Contains(cmd, marcaCogo) && strings.Contains(strings.ToLower(cmd), "cogo")
}
