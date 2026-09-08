package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarAuthorize registra el tool `authorize`. Ver tools.go para lo que comparten.
func registrarAuthorize(s *mcp.Server, d *deps) {
	dir, loadVault, contradictions := d.dir, d.loadVault, d.contradictions
	mcp.AddTool(s, &mcp.Tool{
		Name:        "authorize",
		Description: "Ask whether what you know is enough for what you are about to do. Call it BEFORE any action that changes something outside your own answer — writing files, running migrations, deploying, deleting, sending, publishing. Not every action needs the same backing: explaining something from a yellow note is fine, dropping a table from the same note is not. COGO classifies the action, looks up the confidence this vault requires for that class, and checks the notes you say you are relying on. A NOT AUTHORIZED answer is not an obstacle to route around: report it to the human and let them decide. Declaring a lower class does not lower the bar — the text of the action is classified too and the stricter of the two wins.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in authorizeIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Action) == "" {
			return errResult(fmt.Errorf("authorize needs to know what you are about to do")), nil, nil
		}
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		// El registro compartido, no uno nuevo: abrir un journal lee y parsea
		// todo lo escrito, y esto se llama antes de cada acción de cada agente.
		estados := map[string]confidence.Estado{}
		if j, err := journalDe(dir); err == nil {
			if evs, err := j.All(); err == nil {
				estados, _ = motor.Estados(vault, contradictions(), today(), evs)
			}
		}
		v := accion.Autorizar(
			accion.Peticion{Accion: in.Action, Clase: in.Class, Notas: in.Notes},
			fuenteVault{estados: estados, vault: vault}, pars)
		// Y antes de dejar pasar: mirar a los otros. Una acción impecablemente
		// respaldada sigue siendo un desastre si otro agente la está haciendo
		// ahora mismo.
		v = aplicarChoque(ctx, dir, v, in.Action)
		// Toda consulta queda registrada, autorice o no. Un control que solo deja
		// rastro cuando bloquea no sirve para auditar: lo que se quiere poder
		// reconstruir es en qué se apoyó cada acción, sobre todo las que pasaron.
		_ = appendLog(dir, fmt.Sprintf("authorize %s [%s] %v -> %v",
			v.Clase, v.Necesita, in.Notes, v.Autoriza))
		return textResult(textoAutorizacion(v) + avisoDeOtros(ctx, dir, "")), v, nil
	})
}
