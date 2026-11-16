package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/gancho"
)

// `cogo hook` es lo que el harness corre solo: Claude Code, Codex o Cursor.
// Tres subcomandos:
//
//	session-start   imprime el pack del proyecto; el harness lo agrega al contexto
//	pre-tool        lee la herramienta que está por correr; si es costosa o peor,
//	                le pregunta a COGO, y si COGO no autoriza, se lo pregunta a la
//	                persona donde el harness sabe hacerlo, o la frena
//	post-tool       si se había preguntado y la herramienta corrió, asienta que
//	                la persona la aprobó
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
	_ = fs.String("formato", "", "quién llama: vacío para Claude Code, codex o cursor; si falta, se reconoce por la entrada")
	decide := fs.String("decide", "human", "human: lo que COGO no autoriza lo decide la persona, donde el harness sabe preguntar; cogo: siempre frena")
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
	if s.formato == "" && entrada.Harness() != "claude-code" {
		s.formato = entrada.Harness()
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
		comando, ruta := entrada.Campos()
		resp, err := consultarHook(ctx, *dir, *url, *token, clase, authorizeIn{
			Action: texto, Project: *proyecto, Hook: s.harness(), HookTool: entrada.Tool,
			HookCommand: comando, HookPath: ruta, HookMinimum: string(min),
			HookUseID: entrada.ToolUseID, HookDecide: *decide,
		})
		if err != nil {
			fmt.Fprintln(stderr, "cogo: no se pudo consultar; la acción sigue sin control:", err)
			return s.pasa()
		}
		switch resp.Decision {
		case "deny":
			return s.frena(resp.Motivo)
		case "ask":
			marcarPendiente(entrada.ToolUseID)
			return s.pregunta(resp.Motivo)
		}
		return s.pasa()

	case "post-tool":
		// Solo importa si antes se preguntó: si la herramienta corrió, el humano
		// la aprobó. La marca local evita consultar al servidor en cada llamada.
		if !hayPendiente(entrada.ToolUseID) {
			return 0
		}
		_, err := consultarHook(ctx, *dir, *url, *token, "", authorizeIn{
			Action: entrada.Tool, Hook: s.harness(), HookEvent: "PostToolUse",
			HookTool: entrada.Tool, HookUseID: entrada.ToolUseID,
		})
		if err != nil {
			fmt.Fprintln(stderr, "cogo: no se pudo asentar la aprobación:", err)
			return 0
		}
		borrarPendiente(entrada.ToolUseID)
		return 0
	}
	fmt.Fprintf(stderr, "cogo hook: subcomando desconocido %q\n", sub)
	return s.pasa()
}

// consultarHook le pregunta a COGO como hook: en proceso con el vault local, o
// a la tool authorize del servidor en modo hook. Si el servidor es anterior al
// modo hook y rechaza los campos, se le pregunta como antes, y lo que no
// autoriza se frena.
func consultarHook(ctx context.Context, dir, url, token string, clase accion.Clase, in authorizeIn) (respuestaHook, error) {
	if url == "" {
		conVault(&dir)
		if err := instalarMotor(dir); err != nil {
			return respuestaHook{}, err
		}
		return decidirHook(ctx, nuevasDeps(dir), in), nil
	}
	cs, err := conectarRemoto(ctx, url, token)
	if err != nil {
		return respuestaHook{}, err
	}
	defer cs.Close()
	args := map[string]any{"action": in.Action, "hook": in.Hook, "hook_tool": in.HookTool}
	for k, v := range map[string]string{
		"project": in.Project, "hook_event": in.HookEvent, "hook_command": in.HookCommand, "hook_path": in.HookPath,
		"hook_minimum": in.HookMinimum, "hook_tool_use_id": in.HookUseID, "hook_decide": in.HookDecide,
	} {
		if v != "" {
			args[k] = v
		}
	}
	texto, _, err := llamarRemoto(ctx, cs, "authorize", args)
	if err != nil {
		if in.HookEvent == "PostToolUse" {
			return respuestaHook{}, err
		}
		veredicto, autoriza, err := autorizarDesdeElHook(ctx, "", url, token, in.Project, in.Action, clase)
		if err != nil || autoriza {
			return respuestaHook{}, err
		}
		return respuestaHook{Decision: "deny", Motivo: veredicto}, nil
	}
	var r struct {
		H struct {
			D string `json:"permissionDecision"`
			M string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(texto), &r); err != nil {
		return respuestaHook{}, fmt.Errorf("respuesta del hook: %w", err)
	}
	return respuestaHook{Decision: r.H.D, Motivo: r.H.M}, nil
}

// Las marcas de pendiente viven en el temporal de la máquina del agente: solo
// sirven para no consultar al servidor en cada PostToolUse.
func rutaPendiente(id string) string {
	h := sha256.Sum256([]byte(id))
	return filepath.Join(os.TempDir(), "cogo-pendientes", hex.EncodeToString(h[:12]))
}

func marcarPendiente(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	p := rutaPendiente(id)
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	_ = os.WriteFile(p, nil, 0o600)
}

func hayPendiente(id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	_, err := os.Stat(rutaPendiente(id))
	return err == nil
}

func borrarPendiente(id string) { _ = os.Remove(rutaPendiente(id)) }

// salidaHook le contesta al harness en su formato.
type salidaHook struct {
	formato, sub   string
	stdout, stderr io.Writer
}

func (s salidaHook) cursor() bool { return s.formato == "cursor" }

// harness es el nombre con el que COGO decide si se puede preguntar.
func (s salidaHook) harness() string {
	switch s.formato {
	case "cursor", "codex":
		return s.formato
	}
	return "claude-code"
}

// pregunta: la decisión pasa al humano. Claude Code muestra el diálogo de
// permiso con el motivo; Cursor también, en los comandos de shell. Codex no
// sabe preguntar, y COGO nunca le pide que lo haga: si llegara, frena.
func (s salidaHook) pregunta(motivo string) int {
	switch s.harness() {
	case "cursor":
		s.json(map[string]any{
			"permission":    "ask",
			"user_message":  motivo,
			"agent_message": "COGO le pidió a la persona que decida: " + motivo,
		})
		return 0
	case "codex":
		return s.frena(motivo)
	}
	s.json(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       "ask",
		"permissionDecisionReason": motivo,
	}})
	return 0
}

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
