package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diegoparras/cogo/internal/journal"
)

// EmisorRunner es el emisor reservado, que solo se puede usar por
// journal.AppendEjecucion. El journal rechaza cualquier intento de escribirlo
// por la puerta común, y un grep de AppendEjecucion muestra todos los lugares
// que producen verificaciones — que es la auditoría que una cadena esparcida no
// permite.
const EmisorRunner = journal.EmisorEjecucion

// Verificar ejecuta el check de una nota y deja el rastro en el journal: cuándo
// empezó, qué se corrió, con qué código de salida terminó.
//
// Los dos eventos importan por separado. `VerificationStarted` mueve la nota a
// `verifying`, que la saca del retículo mientras corre — así una verificación en
// curso no arrastra a sus dependientes ni hace parpadear colores. `CheckExecuted`
// es el que decide, y el único que puede producir `verified`.
func Verificar(ctx context.Context, j *journal.Journal, c *Config, noteID, checkID string) (Resultado, error) {
	if c == nil || !c.Habilitado {
		return Resultado{}, ErrDeshabilitado
	}
	if _, ok := c.Buscar(checkID); !ok {
		return Resultado{}, fmt.Errorf("%w: %q", ErrNoAutorizado, checkID)
	}

	if _, err := j.AppendEjecucion(journal.Event{
		NoteID: noteID, Kind: "VerificationStarted",
		Payload: payload(map[string]any{"check": checkID}),
	}); err != nil {
		return Resultado{}, err
	}

	res, err := Ejecutar(ctx, c, checkID)
	if err != nil && !res.PorTimeut {
		// No se pudo observar el check: no hay veredicto que registrar. La nota
		// vuelve sola de `verifying` en el próximo ciclo; inventar un resultado
		// sería peor que no tener ninguno.
		return res, err
	}

	guarda := "ejecucion_ok"
	if !res.OK() {
		guarda = "ejecucion_falla"
	}
	if _, e := j.AppendEjecucion(journal.Event{
		NoteID: noteID, Kind: "CheckExecuted", Guard: guarda,
		Payload: payload(map[string]any{
			"check": res.CheckID, "exit_code": res.ExitCode,
			"duracion_ms": res.Duracion.Milliseconds(), "por_timeout": res.PorTimeut,
			"stdout": res.Stdout, "stderr": res.Stderr,
		}),
	}); e != nil {
		return res, e
	}
	return res, err
}

func payload(v map[string]any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
