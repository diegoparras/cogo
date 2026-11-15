package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegoparras/cogo/internal/gancho"
)

// mezclarMcpServers escribe un servidor en un JSON con `mcpServers` (el
// .mcp.json de Claude Code, el .cursor/mcp.json de Cursor) sin tocar los demás.
func mezclarMcpServers(ruta, nombre string, server map[string]any) error {
	root := map[string]any{}
	if b, err := os.ReadFile(ruta); err == nil {
		_ = json.Unmarshal(b, &root)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[nombre] = server
	root["mcpServers"] = servers
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ruta, append(b, '\n'), 0o644)
}

// pedidoInstall es lo que `cogo install` decidió, para cualquier agente.
type pedidoInstall struct {
	Conf, Nombre, Bin, Vault, URL, Token, Proyecto, Minima string
	Decide                                                 string // "cogo" con -sin-preguntar
	Hooks, ViaMCP                                          bool
}

func (p pedidoInstall) comando() gancho.Comando {
	return gancho.Comando{Binario: p.Bin, Vault: p.Vault, URL: p.URL, Token: p.Token, Proyecto: p.Proyecto, Minima: p.Minima, Decide: p.Decide}
}

func (p pedidoInstall) viaMCP() gancho.ViaMCP {
	return gancho.ViaMCP{Servidor: p.Nombre, Proyecto: p.Proyecto, Minima: p.Minima, Decide: p.Decide}
}

// avisarHooks dice qué quedó escrito y qué hace.
func avisarHooks(ruta, harness string, p pedidoInstall) {
	if p.ViaMCP {
		fmt.Fprintf(os.Stderr, "cogo: wrote %s — %s now calls the `authorize` tool of %q by itself before %s-or-worse actions, and `recall` after compacting; no cogo binary needed on this machine\n", ruta, harness, p.Nombre, p.Minima)
		return
	}
	fmt.Fprintf(os.Stderr, "cogo: wrote %s — %s now runs `cogo hook` by itself (pack on start; authorize before %s-or-worse actions)\n", ruta, harness, p.Minima)
}

// instalarCursor escribe el .cursor/mcp.json y, con hooks, el .cursor/hooks.json
// al lado. El token no se escribe: el mcp.json lo pide como ${env:COGO_TOKEN},
// que Cursor resuelve al cargar, y los hooks lo leen de COGO_TOKEN.
func instalarCursor(p pedidoInstall) error {
	server := map[string]any{"command": p.Bin, "args": []any{"serve", "-vault", p.Vault}}
	modo := "stdio"
	if p.URL != "" {
		modo = "http"
		server = map[string]any{"url": p.URL}
		if p.Token != "" {
			server["headers"] = map[string]any{"Authorization": "Bearer ${env:COGO_TOKEN}"}
		}
	}
	if err := mezclarMcpServers(p.Conf, p.Nombre, server); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "cogo: wired %q → mcpServers.%s (%s)\n", p.Conf, p.Nombre, modo)
	if p.Token != "" {
		fmt.Fprintln(os.Stderr, "cogo: the token was NOT written to disk. Set COGO_TOKEN to it in the environment Cursor starts from.")
	}

	if p.Hooks {
		ruta := filepath.Join(filepath.Dir(p.Conf), "hooks.json")
		previo, _ := os.ReadFile(ruta)
		b, err := gancho.HooksCursor(previo, p.comando())
		if err != nil {
			return err
		}
		if err := os.WriteFile(ruta, b, 0o644); err != nil {
			return err
		}
		avisarHooks(ruta, "Cursor", p)
	}
	return nil
}

// instalarCodex escribe el .codex/config.toml (la conexión MCP) y, con hooks,
// el .codex/hooks.json al lado. Ninguno de los dos lleva el token: Codex lo
// lee de COGO_TOKEN, tanto para el MCP como para los hooks.
func instalarCodex(p pedidoInstall) error {
	s := servidorCodex{Command: p.Bin, Args: []string{"serve", "-vault", p.Vault}}
	modo := "stdio"
	if p.URL != "" {
		modo = "http"
		s = servidorCodex{URL: p.URL}
		if p.Token != "" {
			s.TokenEnv = "COGO_TOKEN"
		}
	}
	previo, _ := os.ReadFile(p.Conf)
	b, err := tomlCodex(previo, p.Nombre, s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.Conf), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p.Conf, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "cogo: wired %q → [mcp_servers.%s] (%s)\n", p.Conf, p.Nombre, modo)
	if p.Token != "" {
		fmt.Fprintln(os.Stderr, "cogo: the token was NOT written to disk. Set COGO_TOKEN to it in the environment Codex starts from.")
	}

	if p.Hooks {
		ruta := filepath.Join(filepath.Dir(p.Conf), "hooks.json")
		previo, _ := os.ReadFile(ruta)
		var b []byte
		if p.ViaMCP {
			b, err = gancho.HooksCodexMCP(previo, p.viaMCP())
		} else {
			b, err = gancho.HooksCodex(previo, p.comando())
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(ruta, b, 0o644); err != nil {
			return err
		}
		avisarHooks(ruta, "Codex", p)
		fmt.Fprintln(os.Stderr, "cogo: Codex loads project hooks only once you trust the project and review the hook.")
	}
	return nil
}

// Lo que `cogo install` escribe para agentes que no son Claude Code.
//
// Codex configura sus servidores MCP en TOML, en .codex/config.toml. COGO no
// trae una librería de TOML para una sola tabla: escribe la suya entre dos
// marcas y, al reinstalar, reemplaza solo lo que está entre ellas.

// servidorCodex es la tabla [mcp_servers.<nombre>] del config.toml de Codex.
// Es stdio (Command + Args) o HTTP (URL); TokenEnv nombra la variable de
// entorno de donde Codex lee el Bearer, porque el token no va al archivo.
type servidorCodex struct {
	Command  string
	Args     []string
	URL      string
	TokenEnv string
}

var claveTOML = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// tomlCodex mezcla la tabla de COGO en un config.toml existente (o vacío).
// Si encuentra una tabla con el mismo nombre escrita a mano, no la pisa: dos
// tablas iguales son un TOML inválido, y borrar la ajena sería peor.
func tomlCodex(existente []byte, nombre string, s servidorCodex) ([]byte, error) {
	if !claveTOML.MatchString(nombre) {
		return nil, fmt.Errorf("nombre de servidor inválido para TOML: %q", nombre)
	}
	inicio := "# >>> cogo mcp_servers." + nombre + ": lo escribe `cogo install -agent codex`; reinstalar lo reemplaza"
	fin := "# <<< cogo mcp_servers." + nombre

	var b strings.Builder
	b.WriteString(inicio + "\n")
	fmt.Fprintf(&b, "[mcp_servers.%s]\n", nombre)
	if s.URL != "" {
		fmt.Fprintf(&b, "url = %s\n", tomlStr(s.URL))
		if s.TokenEnv != "" {
			fmt.Fprintf(&b, "bearer_token_env_var = %s\n", tomlStr(s.TokenEnv))
		}
	} else {
		fmt.Fprintf(&b, "command = %s\n", tomlStr(s.Command))
		args := make([]string, len(s.Args))
		for i, a := range s.Args {
			args[i] = tomlStr(a)
		}
		fmt.Fprintf(&b, "args = [%s]\n", strings.Join(args, ", "))
	}
	b.WriteString(fin)
	bloque := b.String()

	texto := string(existente)
	antes, despues, hayBloque := texto, "", false
	if i := strings.Index(texto, inicio); i >= 0 {
		if j := strings.Index(texto[i:], fin); j >= 0 {
			antes, despues, hayBloque = texto[:i], texto[i+j+len(fin):], true
		}
	}
	aMano := regexp.MustCompile(`(?m)^\s*\[mcp_servers\.` + regexp.QuoteMeta(nombre) + `[\].]`)
	if aMano.MatchString(antes) || aMano.MatchString(despues) {
		return nil, fmt.Errorf("ya hay una tabla [mcp_servers.%s] escrita a mano: borrala o elegí otro -name", nombre)
	}
	if hayBloque {
		return []byte(antes + bloque + despues), nil
	}
	if texto != "" && !strings.HasSuffix(texto, "\n") {
		texto += "\n"
	}
	if texto != "" {
		texto += "\n"
	}
	return []byte(texto + bloque + "\n"), nil
}

// tomlStr escribe un string de TOML. Literal entre comillas simples cuando se
// puede, que no interpreta barras: una ruta de Windows queda tal cual.
func tomlStr(v string) string {
	if !strings.ContainsAny(v, "'\n\r") && !tieneControl(v) {
		return "'" + v + "'"
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func tieneControl(v string) bool {
	for _, r := range v {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return true
		}
	}
	return false
}
