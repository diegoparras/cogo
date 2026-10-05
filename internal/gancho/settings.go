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

	poner := func(evento, matcher, sub string, timeout int) {
		lista, _ := hooks[evento].([]any)
		var limpia []any
		for _, it := range lista {
			if !esNuestro(it) {
				limpia = append(limpia, it)
			}
		}
		entrada := map[string]any{
			"hooks": []any{map[string]any{
				"type": "command", "command": c.linea(sub), "timeout": timeout,
			}},
		}
		if matcher != "" {
			entrada["matcher"] = matcher
		}
		hooks[evento] = append(limpia, entrada)
	}
	// El pack al empezar: entra como contexto sin que nadie lo pida.
	poner("SessionStart", "", "session-start", 30)
	// Y la pregunta antes de tocar algo. Solo las herramientas que cambian
	// cosas afuera de la conversación.
	poner("PreToolUse", "Bash|Edit|Write|MultiEdit|NotebookEdit", "pre-tool", 20)

	root["hooks"] = hooks
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// esNuestro reconoce una entrada de hook escrita por COGO: alguno de sus
// comandos invoca `cogo hook`.
func esNuestro(it any) bool {
	m, ok := it.(map[string]any)
	if !ok {
		return false
	}
	lista, _ := m["hooks"].([]any)
	for _, h := range lista {
		hm, _ := h.(map[string]any)
		cmd, _ := hm["command"].(string)
		if strings.Contains(cmd, marcaCogo) && strings.Contains(strings.ToLower(cmd), "cogo") {
			return true
		}
	}
	return false
}
