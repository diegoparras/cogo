package main

import (
	"context"
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

// `cogo hook` es lo que Claude Code corre solo. Dos subcomandos:
//
//	session-start   imprime el pack del proyecto; Claude Code lo agrega al contexto
//	pre-tool        lee la herramienta que está por correr; si es costosa o peor,
//	                le pregunta a COGO y sale con 2 si no autoriza
//
// # LA REGLA DE SALIDA
//
// Código 0 = sin opinión, la acción sigue. Código 2 = bloqueada, y lo que se
// escriba en stderr es el motivo que ve el modelo. Cualquier otra cosa —el
// servidor caído, un stdin raro— sale 0 con una nota en stderr: un hook que
// bloquea todo cuando COGO no responde deja al agente inutilizable, y se apaga
// a mano en cinco minutos. Falla abierto, y lo dice.
func cmdHook(args []string) int {
	return ejecutarHook(args, os.Stdin, os.Stdout, os.Stderr)
}

// ejecutarHook es cmdHook con los tres streams inyectados, para poder probarlo
// sin procesos ni os.Exit.
func ejecutarHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "uso: cogo hook <session-start|pre-tool> [-vault dir | -url URL -token T] [-project p] [-minima clase]")
		return 0
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("hook "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := vaultFlag(fs)
	url := fs.String("url", os.Getenv("COGO_URL"), "COGO hosteado (endpoint /mcp); si está, gana sobre -vault")
	token := fs.String("token", os.Getenv("COGO_TOKEN"), "Bearer para -url")
	proyecto := fs.String("project", "", "proyecto del pack y del respaldo")
	minima := fs.String("minima", string(accion.Costosa), "clase mínima que dispara la consulta: reversible|costly|irreversible")
	presupuesto := fs.Int("budget", 1500, "tokens del pack de inicio de sesión")
	if err := fs.Parse(args); err != nil {
		return 0
	}
	// Los logs del motor irían a stderr, y en un bloqueo stderr ES el motivo.
	log.SetOutput(io.Discard)

	entrada, err := gancho.Leer(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "cogo:", err)
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch sub {
	case "session-start":
		md, err := packParaSesion(ctx, *dir, *url, *token, *proyecto, *presupuesto)
		if err != nil {
			fmt.Fprintln(stderr, "cogo: no se pudo armar el pack:", err)
			return 0
		}
		fmt.Fprint(stdout, md)
		return 0

	case "pre-tool":
		texto, ok := gancho.Traducir(entrada.Tool, entrada.ToolInput)
		if !ok {
			return 0
		}
		min, valida := accion.Valida(*minima)
		if !valida {
			min = accion.Costosa
		}
		clase, preguntar := gancho.HayQuePreguntar(texto, min)
		if !preguntar {
			return 0
		}
		veredicto, autoriza, err := autorizarDesdeElHook(ctx, *dir, *url, *token, *proyecto, texto, clase)
		if err != nil {
			fmt.Fprintln(stderr, "cogo: no se pudo consultar; la acción sigue sin control:", err)
			return 0
		}
		if autoriza {
			return 0
		}
		fmt.Fprint(stderr, veredicto)
		return 2
	}
	fmt.Fprintf(stderr, "cogo hook: subcomando desconocido %q\n", sub)
	return 0
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
