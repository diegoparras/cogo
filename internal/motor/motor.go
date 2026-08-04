// Package motor arma el color de cada nota con la cadena nueva: los eventos del
// journal se pliegan sobre la máquina de estados, se combinan con los ejes que
// no viven en los eventos (evidencia, frescura, contradicciones), y la duda se
// propaga por el grafo resolviendo un punto fijo.
//
// Devuelve los mismos core.Verdict que el motor anterior. Eso no es casualidad:
// mantener el tipo de salida es lo que permite cambiar el motor sin tocar el
// visor, la CLI ni el servidor MCP — y, sobre todo, poder volver atrás con una
// variable de entorno si algo sale mal.
package motor

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// Legacy dice si hay que usar el motor anterior. Es la marcha atrás: si el
// corte sale mal en una instancia que ya está andando, se arregla con una
// variable de entorno y un reinicio, sin esperar un release.
func Legacy() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COGO_MOTOR"))) {
	case "legacy", "viejo", "old":
		return true
	}
	return false
}

// NoGraduable dice si una nota queda fuera del semáforo. Hoy son los errores
// aprendidos: registran algo que pasó, no afirman algo que haya que creer, así
// que graduarlos por confianza no significa nada.
//
// Quedan fuera del retículo Y de la propagación: ni arrastran a nadie ni son
// arrastradas. Es el mismo criterio que aplicaba el motor anterior, y hacía
// falta traerlo — sin esto, una nota informativa terminaba en rojo.
func NoGraduable(n *core.Note) bool { return n.Type == "mistake" }

// Evaluar calcula el veredicto de cada nota del vault.
func Evaluar(vault map[string]*core.Note, contradicciones map[string]bool, hoy core.Date, evs []journal.Event) map[string]core.Verdict {
	crudos := journal.Fold(evs, time.Time{}, time.Time{})

	local := map[string]confidence.Estado{}
	g := confidence.Grafo{}
	for id, n := range vault {
		if NoGraduable(n) {
			continue // fuera del retículo: no entra al punto fijo
		}
		local[id] = journal.EstadoLocal(crudos[id], n, hoy, contradicciones[id])
		// Una dependencia no graduable tampoco arrastra: se la saca del grafo
		// en vez de tratarla como ausente, que hundiría a quien se apoye en ella.
		var deps []string
		for _, d := range n.DependsOn {
			if dn, existe := vault[d]; existe && NoGraduable(dn) {
				continue
			}
			deps = append(deps, d)
		}
		g[id] = deps
	}
	final := confidence.PuntoFijo(g, local)

	out := make(map[string]core.Verdict, len(vault))
	for id, n := range vault {
		if NoGraduable(n) {
			out[id] = core.Verdict{
				Color:  core.Ungraded,
				Reason: "error registrado: es informativo, no se gradúa por confianza",
			}
			continue
		}
		est := final[id]
		out[id] = core.Verdict{
			Color:   color(est),
			Reason:  razon(est, n, local[id]),
			StaleAt: n.StaleAt,
		}
	}
	return out
}

func color(e confidence.Estado) core.Color {
	switch e.Color() {
	case "green":
		return core.Green
	case "yellow":
		return core.Yellow
	default:
		return core.Red
	}
}

// razon explica el veredicto en una frase. No se genera desde el estado a secas:
// cuando una nota cayó por una dependencia, lo que hay que decir es cuál — si no,
// el usuario ve "rojo" sin saber dónde mirar.
func razon(final confidence.Estado, n *core.Note, propio confidence.Estado) string {
	if final != propio {
		return fmt.Sprintf("%s — y además depende de una nota más débil", explicar(propio))
	}
	return explicar(final)
}

func explicar(e confidence.Estado) string {
	switch e {
	case confidence.Verified:
		return "el check se ejecutó y pasó"
	case confidence.ClaimedPassed:
		return "hay evidencia observada y alguien declaró que el check pasa"
	case confidence.CheckDeclared:
		return "hay un criterio de verificación, pero todavía no se ejecutó ni se declaró"
	case confidence.Asserted:
		return "capturada, sin criterio de verificación declarado"
	case confidence.Stale:
		return "venció su ventana de frescura: hay que re-verificarla"
	case confidence.Contradicted:
		return "sin evidencia suficiente, o con una contradicción abierta"
	case confidence.Refuted:
		return "el check se ejecutó y falló"
	case confidence.Quarantined:
		return "en cuarentena: excluida a propósito"
	case confidence.Verifying:
		return "verificándose ahora mismo"
	}
	return "sin determinar"
}
