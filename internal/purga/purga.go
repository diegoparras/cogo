// Package purga borra de verdad: la nota de la papelera, la salida del runner
// en el registro, y los artefactos que ya nadie cita.
//
// # POR QUÉ NO ALCANZA CON LA PAPELERA
//
// Vaciar la papelera borraba el archivo de la nota. Pero el registro guarda lo
// que el runner imprimió al correr sus checks —y ahí puede haber quedado un
// secreto o un dato personal— y el almacén de artefactos guarda lo que la nota
// citaba por hash. Purgar es la única operación que toca las tres cosas, y
// deja en el registro la marca de que se hizo (ver journal.Purgar: rompe la
// cadena a propósito y lo dice).
package purga

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegoparras/cogo/internal/artifact"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// Resultado es lo que se le cuenta a quien purgó.
type Resultado struct {
	Nota               string   `json:"nota"`
	ArchivoBorrado     bool     `json:"archivo_borrado"`     // estaba en la papelera y se borró
	EventosReescritos  []uint64 `json:"eventos_reescritos"`  // los que llevaban salida del runner
	CabezaAnterior     string   `json:"cabeza_anterior"`     // la que tenía el registro antes
	EventoPurged       uint64   `json:"evento_purged"`       // el que quedó asentado
	ArtefactosBorrados int      `json:"artefactos_borrados"` // los que ya nadie citaba
	CadenaRota         bool     `json:"cadena_rota"`         // true si se reescribió algo
}

// Purgar borra la nota `id` de la papelera (si está ahí), saca la salida del
// runner de sus eventos en el registro y asienta el Purged, y por último tira
// los artefactos que ya nadie cita. Una nota VIVA no se purga: primero se
// manda a la papelera (remove), para que borrar de verdad sea siempre dos
// pasos y nunca uno. Si la nota no está ni viva ni en la papelera, igual se
// limpia el registro: es el caso de la que ya se vació antes de que existiera
// esto.
func Purgar(ctx context.Context, dir string, j *journal.Journal, id, quien, motivo string) (Resultado, error) {
	id = strings.TrimSpace(id)
	if err := core.ValidarID(id); err != nil {
		return Resultado{}, err
	}
	if _, err := os.Stat(filepath.Join(dir, id+".md")); err == nil {
		return Resultado{}, fmt.Errorf("purga: %q está viva; primero mandala a la papelera (remove) y después purgala", id)
	}
	if j == nil {
		var err error
		if j, err = journal.Open(dir); err != nil {
			return Resultado{}, err
		}
	}
	res := Resultado{Nota: id}

	// Los artefactos se anotan ANTES de borrar la nota: después no hay de dónde
	// leerlos.
	var shas []string
	if n, err := core.ReadTrashNote(dir, id); err == nil {
		shas = core.ArtifactRefs(n)
		if err := core.PurgeTrash(dir, id); err != nil {
			return res, err
		}
		res.ArchivoBorrado = true
	}

	p, ev, err := j.Purgar(id, quien, motivo)
	if err != nil {
		return res, err
	}
	res.EventosReescritos, res.CabezaAnterior, res.EventoPurged = p.Eventos, p.CabezaAnterior, ev.Seq
	res.CadenaRota = len(p.Eventos) > 0

	// El almacén está deduplicado: un blob puede respaldar varias notas, así
	// que solo se tira el que ya nadie —vivo o en la papelera— cita.
	if len(shas) > 0 {
		keep := core.ReferencedArtifacts(dir)
		store := artifact.FromEnv(dir)
		for _, sha := range shas {
			if keep[sha] {
				continue
			}
			if err := store.Delete(ctx, sha); err == nil {
				res.ArtefactosBorrados++
			}
		}
	}
	return res, nil
}
