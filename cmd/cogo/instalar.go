package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegoparras/cogo/internal/gancho"
)

// instalarCodex escribe el .codex/config.toml (la conexión MCP) y, con hooks,
// el .codex/hooks.json al lado. Ninguno de los dos lleva el token: Codex lo
// lee de COGO_TOKEN, tanto para el MCP como para los hooks.
func instalarCodex(conf, nombre, bin, vault, url, token string, hooks bool, proyecto, minima string) error {
	s := servidorCodex{Command: bin, Args: []string{"serve", "-vault", vault}}
	modo := "stdio"
	if url != "" {
		modo = "http"
		s = servidorCodex{URL: url}
		if token != "" {
			s.TokenEnv = "COGO_TOKEN"
		}
	}
	previo, _ := os.ReadFile(conf)
	b, err := tomlCodex(previo, nombre, s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(conf, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "cogo: wired %q → [mcp_servers.%s] (%s)\n", conf, nombre, modo)
	if token != "" {
		fmt.Fprintln(os.Stderr, "cogo: the token was NOT written to disk. Set COGO_TOKEN to it in the environment Codex starts from.")
	}

	if hooks {
		p := filepath.Join(filepath.Dir(conf), "hooks.json")
		previo, _ := os.ReadFile(p)
		b, err := gancho.HooksCodex(previo, gancho.Comando{
			Binario: bin, Vault: vault, URL: url, Proyecto: proyecto, Minima: minima,
		})
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "cogo: wrote %s — Codex now runs `cogo hook` by itself (pack on start; authorize before %s-or-worse actions)\n", p, minima)
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
