package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/decision"
	"github.com/diegoparras/cogo/internal/gancho"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/diegoparras/cogo/internal/recibo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarAuthorize registra el tool `authorize`. Ver tools.go para lo que comparten.
func registrarAuthorize(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "authorize",
		Description: "Ask whether what you know is enough for what you are about to do. Call it BEFORE any action that changes something outside your own answer — writing files, running migrations, deploying, deleting, sending, publishing. Not every action needs the same backing: explaining something from a yellow note is fine, dropping a table from the same note is not. COGO classifies the action, looks up the confidence this vault requires for that class, and checks the notes you say you are relying on. A NOT AUTHORIZED answer is not an obstacle to route around: report it to the human and let them decide. Declaring a lower class does not lower the bar — the text of the action is classified too and the stricter of the two wins. If you cite no notes, pass find_support:true and COGO will look for the notes that bear on the action itself.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in authorizeIn) (*mcp.CallToolResult, any, error) {
		if in.Hook != "" {
			return textResult(autorizarComoHook(ctx, d, in)), nil, nil
		}
		if strings.TrimSpace(in.Action) == "" {
			return errResult(fmt.Errorf("authorize needs to know what you are about to do")), nil, nil
		}
		// La vuelta de una pregunta: la persona ya contestó.
		if len(req.Params.InputResponses) > 0 {
			return respuestaDeLaPersona(ctx, d, req, in), nil, nil
		}
		v, texto, err := decidirAutorizacion(ctx, d, in)
		if err != nil {
			return errResult(err), nil, nil
		}
		// Lo que COGO no autoriza, si el cliente sabe preguntar, lo decide la
		// persona. La pregunta viaja como un pedido de entrada en el resultado,
		// y el cliente vuelve a llamar con la respuesta.
		if id := reciboDe(texto); !v.Autoriza && id != "" && sabePreguntar(req) {
			_ = decision.Abrir(d.dir).Pedir(decision.Pedido{Recibo: id, Accion: in.Action, Via: decision.ViaPregunta, Cuando: time.Now().UTC()})
			return &mcp.CallToolResult{
				InputRequests: mcp.InputRequestMap{clavePregunta: &mcp.ElicitParams{
					Mode:            "form",
					Message:         mensajeParaLaPersona(in.Action, v.Porque, id),
					RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{}},
				}},
				RequestState: id,
			}, nil, nil
		}
		return textResult(texto), v, nil
	})
}

const clavePregunta = "cogo_aprobacion"

// sabePreguntar dice si el cliente puede mostrarle un formulario a la persona.
// Un cliente de la spec anterior sobre HTTP sin estado no puede: el servidor
// no tiene sesión desde la cual preguntarle, y ahí no declara la capacidad.
func sabePreguntar(req *mcp.CallToolRequest) bool {
	caps := req.ClientCapabilities()
	if caps == nil || caps.Elicitation == nil {
		return false
	}
	e := caps.Elicitation
	return e.Form != nil || e.URL == nil
}

func mensajeParaLaPersona(accion, porque, recibo string) string {
	return "COGO no autoriza esta acción del agente:\n\n    " + accion + "\n\n" + strings.TrimSpace(porque) +
		"\n\n¿La autorizás vos? Si aceptás, queda asentado que la decidiste vos. Recibo " + recibo + "."
}

// respuestaDeLaPersona cierra la pregunta. El RequestState lo devuelve el
// cliente, así que solo vale si nombra una pregunta abierta sobre esta misma
// acción: un cliente no puede aprobar otra cosa cambiando el número.
func respuestaDeLaPersona(ctx context.Context, d *deps, req *mcp.CallToolRequest, in authorizeIn) *mcp.CallToolResult {
	id := req.Params.RequestState
	ahora := time.Now().UTC()
	decisiones := decision.Abrir(d.dir)
	abierta, ok := decisiones.Estados(ahora)[id]
	if !ok || abierta.Estado != decision.Pendiente || abierta.Via != decision.ViaPregunta || abierta.Accion != in.Action {
		return errResult(fmt.Errorf("authorize: the answer does not match any open question"))
	}
	porque := ""
	if r, hay := recibo.Abrir(d.dir).Buscar(id); hay {
		porque = r.Porque
	}
	quien := auth.CallerCtx(ctx)
	if res, _ := req.Params.InputResponses[clavePregunta].(*mcp.ElicitResult); res != nil && res.Action == "accept" {
		_ = decisiones.Resolver(id, decision.Aprobada, quien, ahora)
		return textResult(fmt.Sprintf("AUTHORIZED BY THE PERSON — %s\n\nCOGO did not authorize it: %s\nThe person was asked and approved it. It is on record as their decision, not COGO's.\n\nreceipt: %s",
			in.Action, porque, id))
	}
	_ = decisiones.Resolver(id, decision.Rechazada, quien, ahora)
	return textResult(fmt.Sprintf("NOT AUTHORIZED — the person was asked and did not approve it.\n\n%s\n\nDo not proceed. Reporting the block is a valid outcome; working around it is not.\n\nreceipt: %s",
		porque, id))
}

// autorizarComoHook es authorize llamado por el hook del harness y no por el
// agente. Contesta el JSON de PreToolUse que leen Claude Code y Codex; el
// hook de Cursor lo traduce al suyo.
func autorizarComoHook(ctx context.Context, d *deps, in authorizeIn) string {
	return decidirHook(ctx, d, in).json()
}

// respuestaHook es lo que COGO le contesta a un hook: nada, frenar o preguntar.
type respuestaHook struct {
	Decision string // "" sin objeción, "deny" o "ask"
	Motivo   string
	Recibo   string
}

func (r respuestaHook) json() string {
	if r.Decision == "" {
		return "{}"
	}
	b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       r.Decision,
		"permissionDecisionReason": r.Motivo,
	}})
	return string(b)
}

// decidirHook traduce la herramienta, aplica la clase mínima igual que `cogo
// hook pre-tool` y decide. Cuando COGO no autoriza y el harness sabe
// preguntar, la decisión pasa al humano y queda pendiente hasta que el
// PostToolUse confirme que se ejecutó. Sin objeción, y ante cualquier error,
// no dice nada: el hook falla abierto.
func decidirHook(ctx context.Context, d *deps, in authorizeIn) respuestaHook {
	decisiones := decision.Abrir(d.dir)
	ahora := time.Now().UTC()
	if in.HookEvent == "PostToolUse" {
		// Si llegó acá, la herramienta corrió: el humano la aprobó.
		if r, ok := decisiones.PorToolUse(in.HookUseID); ok {
			_ = decisiones.Resolver(r, decision.Aprobada, auth.CallerCtx(ctx), ahora)
		}
		return respuestaHook{}
	}
	entrada := map[string]any{"command": in.HookCommand, "file_path": in.HookPath, "notebook_path": in.HookPath}
	texto, ok := gancho.Traducir(in.HookTool, entrada)
	// Un placeholder que el harness no resolvió no es una acción. Una variable
	// de shell como ${BUILD} sí lo es, y se juzga.
	if !ok || strings.Contains(texto, "${tool_input") || strings.Contains(texto, "${tool_name") {
		return respuestaHook{}
	}
	min, valida := accion.Valida(in.HookMinimum)
	if !valida {
		min = accion.Costosa
	}
	clase, preguntar := gancho.HayQuePreguntar(texto, min)
	if !preguntar {
		return respuestaHook{}
	}
	v, veredicto, err := decidirAutorizacion(ctx, d, authorizeIn{
		Action: texto, Class: string(clase), Project: in.Project, FindSupport: true,
	})
	if err != nil || v.Autoriza {
		return respuestaHook{}
	}
	recibo := reciboDe(veredicto)
	if !puedePreguntar(in) {
		return respuestaHook{Decision: "deny", Motivo: veredicto, Recibo: recibo}
	}
	if recibo != "" {
		_ = decisiones.Pedir(decision.Pedido{Recibo: recibo, ToolUseID: in.HookUseID, Accion: texto, Via: decision.ViaHook, Cuando: ahora})
	}
	return respuestaHook{Decision: "ask", Motivo: motivoParaElHumano(v.Porque, recibo), Recibo: recibo}
}

// puedePreguntar dice si el harness sabe pedirle permiso al humano. Codex no
// tiene `ask`: un hook que lo contesta se marca como fallido y la acción
// sigue, así que ahí COGO frena. Cursor lo respeta en los comandos de shell,
// no en las ediciones.
func puedePreguntar(in authorizeIn) bool {
	if in.HookDecide == "cogo" {
		return false
	}
	switch in.Hook {
	case "claude-code":
		return true
	case "cursor":
		return in.HookTool == "Bash" || in.HookTool == "Shell"
	}
	return false
}

// motivoParaElHumano es lo que se lee en el diálogo de permiso.
func motivoParaElHumano(porque, recibo string) string {
	porque = strings.TrimSpace(porque)
	if resto, ok := strings.CutPrefix(porque, "COGO "); ok {
		porque = resto // "COGO no lo autoriza: no encontró…", no "…: COGO no encontró…"
	}
	m := "COGO no lo autoriza: " + porque
	if recibo != "" {
		m += " Recibo " + recibo + "."
	}
	return m + " Si lo aprobás, queda asentado que lo decidiste vos."
}

var patronRecibo = regexp.MustCompile(`receipt: (r-\d{8}-\d{6}-[0-9a-f]{6})`)

// reciboDe saca el id del recibo del texto del veredicto.
func reciboDe(veredicto string) string {
	if m := patronRecibo.FindStringSubmatch(veredicto); m != nil {
		return m[1]
	}
	return ""
}

// decidirAutorizacion es la política entera de `authorize`, separada del tool
// para que el hook la use sin levantar un servidor.
func decidirAutorizacion(ctx context.Context, d *deps, in authorizeIn) (accion.Veredicto, string, error) {
	vault, err := d.loadVault()
	if err != nil {
		return accion.Veredicto{}, "", err
	}
	cx := d.contradictions()
	// Evaluar primero siembra el registro con las notas que todavía no conoce.
	// Sin esto, un `authorize` antes de cualquier `pack` en un proceso recién
	// arrancado veía todas las notas como si no tuvieran historia.
	_ = core.EvaluateVault(vault, cx, today())
	// El registro compartido, no uno nuevo: abrir un journal lee y parsea
	// todo lo escrito, y esto se llama antes de cada acción de cada agente.
	estados := map[string]confidence.Estado{}
	if j, err := journalDe(d.dir); err == nil {
		if evs, err := j.All(); err == nil {
			estados, _ = motor.Estados(vault, cx, today(), evs)
		}
	}

	// Sin notas citadas y con find_support, COGO busca en qué se apoyaría: el
	// pack sobre el texto de la acción. Solo cuentan las que no son rojas —lo
	// rojo ya está en cuarentena— ni preguntas abiertas.
	var buscadas []string
	busco := len(limpias(in.Notes)) == 0 && in.FindSupport
	if busco {
		p := core.BuildPack(vault, cx, core.PackOptions{Query: in.Action, Project: in.Project, Budget: 3000, Today: today()})
		for _, id := range p.Incluidas {
			n, ok := vault[id]
			if !ok || core.EsBrecha(n) {
				continue
			}
			if e, ok := estados[id]; ok && e.Color() != "red" {
				buscadas = append(buscadas, id)
			}
		}
		in.Notes = buscadas
	}
	// Y de lo citado o encontrado, solo cuenta lo que HABLA de la acción. Una
	// nota verde sobre Redis no respalda borrar la tabla de usuarios. Es el
	// único filtro semántico del autorizador, y solo saca.
	var noPertinentes []string
	var juicios map[string]float64
	in.Notes, noPertinentes, juicios = filtrarPertinentes(ctx, vault, in.Notes, in.Action)

	v := accion.Autorizar(
		accion.Peticion{Accion: in.Action, Clase: in.Class, Notas: in.Notes},
		fuenteVault{estados: estados, vault: vault}, pars)
	if busco && len(buscadas) == 0 && !v.Autoriza {
		v.Porque = "COGO no encontró ninguna nota que hable de esto. Una acción " + accion.Rotulo[accion.Clase(v.Clase)] +
			" sobre algo de lo que el vault no sabe nada no se autoriza: capturá lo que comprobaste, verificalo, o pedile al humano."
	}
	// Y antes de dejar pasar: mirar a los otros. Una acción impecablemente
	// respaldada sigue siendo un desastre si otro agente la está haciendo
	// ahora mismo.
	v = aplicarChoque(ctx, d.dir, v, in.Action)
	// Toda consulta queda registrada, autorice o no. Un control que solo deja
	// rastro cuando bloquea no sirve para auditar: lo que se quiere poder
	// reconstruir es en qué se apoyó cada acción, sobre todo las que pasaron.
	_ = appendLog(d.dir, fmt.Sprintf("authorize %s [%s] %v -> %v",
		v.Clase, v.Necesita, in.Notes, v.Autoriza))
	// Y el recibo: qué sabía el agente cuando pidió, atado a la cabeza del
	// registro. Es lo que después permite preguntar "¿qué sabía a las 15:32?"
	// y obtener una respuesta exacta, no una reconstrucción de memoria.
	idRecibo := registrarRecibo(ctx, d, in, v, estados, noPertinentes, juicios)

	texto := textoAutorizacion(v)
	if idRecibo != "" {
		texto += "\n\nreceipt: " + idRecibo + " — cite it when you report this to the human"
	}
	if len(noPertinentes) > 0 {
		texto += "\n\nnot counted as support (Jev: they do not bear on this action): " + strings.Join(noPertinentes, ", ")
	}
	if busco {
		if len(buscadas) > 0 {
			texto += "\n\nsupport found by COGO: " + strings.Join(buscadas, ", ")
		} else {
			texto += "\n\nsupport found by COGO: none"
		}
	}
	return v, texto + avisoDeOtros(ctx, d.dir, in.Project), nil
}

func limpias(ids []string) []string {
	var out []string
	for _, id := range ids {
		if s := strings.TrimSpace(id); s != "" {
			out = append(out, s)
		}
	}
	return out
}
