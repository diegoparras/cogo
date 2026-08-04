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

// NoGraduable dice si una nota queda fuera del semáforo.
//
// Son dos casos, y por la misma razón: no afirman nada que haya que creer.
//
//	mistake — registra algo que pasó. Es informativo.
//	gap     — es una PREGUNTA ABIERTA. Declara que algo no se sabe.
//
// Que una brecha no tenga color es la parte importante y la más fácil de hacer
// mal. Sería tentador pintarla de rojo —no hay evidencia, después de todo— pero
// eso confundiría dos cosas distintas: una nota roja AFIRMA algo sin respaldo,
// una brecha no afirma nada. Ponerle color la volvería una mala afirmación en
// vez de una buena pregunta.
//
// Quedan fuera del retículo Y de la propagación: ni arrastran a nadie ni son
// arrastradas.
func NoGraduable(n *core.Note) bool {
	return n.Type == "mistake" || core.EsBrecha(n)
}

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
			out[id] = core.Verdict{Color: core.Ungraded, Reason: razonNoGraduable(n)}
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

func razonNoGraduable(n *core.Note) string {
	if core.EsBrecha(n) {
		if k := len(n.Blocks); k > 0 {
			return fmt.Sprintf("pregunta abierta: no se sabe todavía, y hay %d decisión(es) esperándola", k)
		}
		return "pregunta abierta: es algo que el proyecto no sabe, no una afirmación"
	}
	return "error registrado: es informativo, no se gradúa por confianza"
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
	base := explicar(propio)
	// La deriva material entra al retículo como un techo, y el techo a secas
	// dice "todavía no se ejecutó ni se declaró" — que en este caso es falso: se
	// declaró, y después cambió el archivo. Decir cuál es lo que convierte el
	// amarillo en algo que se puede arreglar.
	if propio == confidence.CheckDeclared && core.HayDerivaMaterial(n) {
		base = fmt.Sprintf("cambió lo que la nota cita (%s): la evidencia ya no la respalda igual",
			strings.Join(core.DriftedRefs(n), ", "))
	}
	if final != propio {
		return base + " — y además depende de una nota más débil"
	}
	return base
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
