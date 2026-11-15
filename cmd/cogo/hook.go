package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/gancho"
)

// `cogo hook` es lo que el harness corre solo: Claude Code, Codex o Cursor.
// Dos subcomandos:
//
//	session-start   imprime el pack del proyecto; el harness lo agrega al contexto
//	pre-tool        lee la herramienta que está por correr; si es costosa o peor,
//	                le pregunta a COGO y la frena si no autoriza
//
// # LA REGLA DE SALIDA
//
// Claude Code y Codex comparten contrato: código 0 = sin opinión, la acción
// sigue; código 2 = bloqueada, y lo que se escriba en stderr es el motivo que
// ve el modelo. Con `-formato cursor` todo va en JSON por stdout, porque Cursor
// bloquea la acción si un hook de permiso no contesta un JSON válido.
//
// Cualquier otra cosa —el servidor caído, un stdin raro— pasa, con una nota en
// stderr: un hook que bloquea todo cuando COGO no responde deja al agente
// inutilizable, y se apaga a mano en cinco minutos. Falla abierto, y lo dice.
func cmdHook(args []string) int {
	return ejecutarHook(args, os.Stdin, os.Stdout, os.Stderr)
}

// ejecutarHook es cmdHook con los tres streams inyectados, para poder probarlo
// sin procesos ni os.Exit.
func ejecutarHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// El formato se lee antes que nada: hasta un error de flags tiene que
	// contestarse en el formato que el harness espera.
	s := salidaHook{formato: formatoDe(args), stdout: stdout, stderr: stderr}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "uso: cogo hook <session-start|pre-tool> [-vault dir | -url URL -token T] [-project p] [-minima clase] [-formato cursor]")
		return s.pasa()
	}
	sub, args := args[0], args[1:]
	s.sub = sub
	fs := flag.NewFlagSet("hook "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := vaultFlag(fs)
	url := fs.String("url", os.Getenv("COGO_URL"), "COGO hosteado (endpoint /mcp); si está, gana sobre -vault")
	token := fs.String("token", os.Getenv("COGO_TOKEN"), "Bearer para -url")
	proyecto := fs.String("project", "", "proyecto del pack y del respaldo")
	minima := fs.String("minima", string(accion.Costosa), "clase mínima que dispara la consulta: reversible|costly|irreversible")
	presupuesto := fs.Int("budget", 1500, "tokens del pack de inicio de sesión")
	_ = fs.String("formato", "", "cómo contestar: vacío para Claude Code y Codex, cursor para Cursor")
	if err := fs.Parse(args); err != nil {
		return s.pasa()
	}
	// Los logs del motor irían a stderr, y en un bloqueo stderr ES el motivo.
	log.SetOutput(io.Discard)

	entrada, err := gancho.Leer(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "cogo:", err)
		return s.pasa()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch sub {
	case "session-start":
		md, err := packParaSesion(ctx, *dir, *url, *token, *proyecto, *presupuesto)
		if err != nil {
			fmt.Fprintln(stderr, "cogo: no se pudo armar el pack:", err)
			return s.pasa()
		}
		return s.contexto(md)

	case "pre-tool":
		texto, ok := gancho.Traducir(entrada.Tool, entrada.ToolInput)
		if !ok {
			return s.pasa()
		}
		min, valida := accion.Valida(*minima)
		if !valida {
			min = accion.Costosa
		}
		clase, preguntar := gancho.HayQuePreguntar(texto, min)
		if !preguntar {
			return s.pasa()
		}
		veredicto, autoriza, err := autorizarDesdeElHook(ctx, *dir, *url, *token, *proyecto, texto, clase)
		if err != nil {
			fmt.Fprintln(stderr, "cogo: no se pudo consultar; la acción sigue sin control:", err)
			return s.pasa()
		}
		if autoriza {
			return s.pasa()
		}
		return s.frena(veredicto)
	}
	fmt.Fprintf(stderr, "cogo hook: subcomando desconocido %q\n", sub)
	return s.pasa()
}

// salidaHook le contesta al harness en su formato.
type salidaHook struct {
	formato, sub   string
	stdout, stderr io.Writer
}

func (s salidaHook) cursor() bool { return s.formato == "cursor" }

// pasa: COGO no tiene objeción. En Claude Code y Codex es no decir nada; en
// Cursor hay que decirlo, porque un hook de permiso sin JSON bloquea.
func (s salidaHook) pasa() int {
	if s.cursor() {
		if s.sub == "session-start" {
			s.json(map[string]any{})
		} else {
			s.json(map[string]any{"permission": "allow"})
		}
	}
	return 0
}

// frena: la acción no sale, y el modelo lee el veredicto entero.
func (s salidaHook) frena(veredicto string) int {
	if s.cursor() {
		s.json(map[string]any{
			"permission":    "deny",
			"user_message":  "COGO: " + primeraLinea(veredicto),
			"agent_message": veredicto,
		})
		return 0
	}
	fmt.Fprint(s.stderr, veredicto)
	return 2
}

// contexto: lo que entra al arrancar la sesión.
func (s salidaHook) contexto(md string) int {
	if s.cursor() {
		s.json(map[string]any{"additional_context": md})
		return 0
	}
	fmt.Fprint(s.stdout, md)
	return 0
}

func (s salidaHook) json(v any) {
	b, _ := json.Marshal(v)
	_, _ = s.stdout.Write(append(b, '\n'))
}

// formatoDe busca -formato en los argumentos sin parsearlos, para poder
// contestar bien aunque el resto de los flags venga roto.
func formatoDe(args []string) string {
	for i, a := range args {
		a = strings.TrimPrefix(a, "-")
		if v, ok := strings.CutPrefix(a, "-formato="); ok {
			return v
		}
		if v, ok := strings.CutPrefix(a, "formato="); ok {
			return v
		}
		if a == "formato" || a == "-formato" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

func primeraLinea(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// packParaSesion arma el pack de arranque, local o remoto.
func packParaSesion(ctx context.Context, dir, url, token, proyecto string, presupuesto int) (string, error) {
	if url != "" {
		cs, err := conectarRemoto(ctx, url, token)
		if err != nil {
			return "", err
		}
		defer cs.Close()
		texto, _, err := llamarRemoto(ctx, cs, "pack", map[string]any{
			"query": "", "project": proyecto, "token_budget": presupuesto,
		})
		return texto, err
	}
	conVault(&dir)
	vault, err := core.LoadVault(dir)
	if err != nil {
		return "", err
	}
	core.ResolveEvidence(vault, core.LoadEvidenceRoots(dir))
	p := core.BuildPack(vault, nil, core.PackOptions{Project: proyecto, Budget: presupuesto, Today: today()})
	return p.Markdown, nil
}

// autorizarDesdeElHook pregunta con `find_support`: el agente no citó notas
// —está corriendo un comando— así que COGO busca solo en qué se apoyaría.
func autorizarDesdeElHook(ctx context.Context, dir, url, token, proyecto, texto string, clase accion.Clase) (string, bool, error) {
	if url != "" {
		cs, err := conectarRemoto(ctx, url, token)
		if err != nil {
			return "", false, err
		}
		defer cs.Close()
		veredicto, est, err := llamarRemoto(ctx, cs, "authorize", map[string]any{
			"action": texto, "class": string(clase), "project": proyecto, "find_support": true,
		})
		if err != nil {
			return "", false, err
		}
		autoriza, _ := est["autoriza"].(bool)
		if est == nil {
			autoriza = !strings.HasPrefix(veredicto, "NOT AUTHORIZED")
		}
		return veredicto, autoriza, nil
	}
	conVault(&dir)
	if err := instalarMotor(dir); err != nil {
		return "", false, err
	}
	d := nuevasDeps(dir)
	v, veredicto, err := decidirAutorizacion(ctx, d, authorizeIn{
		Action: texto, Class: string(clase), Project: proyecto, FindSupport: true,
	})
	if err != nil {
		return "", false, err
	}
	return veredicto, v.Autoriza, nil
}
