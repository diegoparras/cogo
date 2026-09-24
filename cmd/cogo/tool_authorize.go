package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarAuthorize registra el tool `authorize`. Ver tools.go para lo que comparten.
func registrarAuthorize(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "authorize",
		Description: "Ask whether what you know is enough for what you are about to do. Call it BEFORE any action that changes something outside your own answer — writing files, running migrations, deploying, deleting, sending, publishing. Not every action needs the same backing: explaining something from a yellow note is fine, dropping a table from the same note is not. COGO classifies the action, looks up the confidence this vault requires for that class, and checks the notes you say you are relying on. A NOT AUTHORIZED answer is not an obstacle to route around: report it to the human and let them decide. Declaring a lower class does not lower the bar — the text of the action is classified too and the stricter of the two wins. If you cite no notes, pass find_support:true and COGO will look for the notes that bear on the action itself.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in authorizeIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Action) == "" {
			return errResult(fmt.Errorf("authorize needs to know what you are about to do")), nil, nil
		}
		v, texto, err := decidirAutorizacion(ctx, d, in)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textResult(texto), v, nil
	})
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
	in.Notes, noPertinentes = filtrarPertinentes(ctx, vault, in.Notes, in.Action)

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

	texto := textoAutorizacion(v)
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
