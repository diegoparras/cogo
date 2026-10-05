package supervivencia

import (
	"time"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// Observar traduce el vault y su journal a observaciones de supervivencia.
//
// # CUÁNDO EMPIEZA EL RELOJ
//
// En `last_verified`. Es la definición correcta y no la obvia: no interesa
// cuándo se creó el archivo de la nota, interesa desde cuándo se sabe que lo que
// afirma es cierto. Una nota escrita en enero y re-verificada en julio empezó a
// correr en julio — es una afirmación de julio.
//
// Es, además, exactamente la cantidad que una ventana de frescura quiere
// modelar: cuánto tiempo después de verificar algo sigue siendo verdad. La
// ventana se estima sobre lo mismo que gobierna.
//
// # CUÁNDO SE MUERE
//
// Con la primera señal de que la afirmación dejó de valer: un check ejecutado
// que falló, o una contradicción abierta. Nada más cuenta como muerte —que
// venza la ventana NO es una muerte, es justamente lo que se está tratando de
// predecir, y contarlo sería estimar el parámetro con su propio resultado.
//
// Lo que no murió está censurado a hoy: no se sabe cuánto va a durar, pero se
// sabe que llegó hasta acá, y eso es información que Kaplan-Meier usa.
func Observar(vault map[string]*core.Note, evs []journal.Event, hoy time.Time) []Observacion {
	muerte := map[string]time.Time{}
	for _, ev := range evs {
		fatal := (ev.Kind == "CheckExecuted" && ev.Guard == "ejecucion_falla") ||
			ev.Kind == "ContradictionOpened" || ev.Kind == "Refuted"
		if !fatal {
			continue
		}
		cuando := ev.ValidTime
		if cuando.IsZero() {
			cuando = ev.TxTime
		}
		if cuando.IsZero() {
			continue
		}
		// La primera muerte manda: lo que pasó después ya fue con la nota caída.
		if anterior, hay := muerte[ev.NoteID]; !hay || cuando.Before(anterior) {
			muerte[ev.NoteID] = cuando
		}
	}

	var out []Observacion
	for id, n := range vault {
		if n.Type == "mistake" || core.EsBrecha(n) {
			continue // no se gradúan: no tienen frescura que estimar
		}
		if n.LastVerified.IsZero() {
			continue // nunca se verificó: el reloj no arrancó
		}
		nacio := n.LastVerified.Time()
		fin, murio := muerte[id]
		if !murio {
			fin = hoy
		}
		dias := int(fin.Sub(nacio).Hours() / 24)
		if dias < 0 {
			continue // una muerte anterior a la última verificación: se re-verificó después, sigue viva
		}
		out = append(out, Observacion{Tipo: n.Type, Dias: dias, Fallo: murio})
	}
	return out
}
