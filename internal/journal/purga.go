package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/atomico"
)

// Purgar: la única escritura que no es un append.
//
// # POR QUÉ EXISTE
//
// El registro guarda lo que el runner imprimió al correr cada check. Eso es lo
// que hace que `verified` valga algo: se puede volver a leer qué salió. Pero
// también es donde termina un secreto que un comando escupió por stdout, o un
// dato personal de una base de pruebas. Borrar la nota no lo saca de ahí.
//
// La respuesta honesta no es "el registro es inmutable, lo siento": es borrar
// lo que hay que borrar y que quede la marca de que se borró. Purgar reescribe
// los eventos de esa nota que llevan salida del runner —saca stdout y stderr
// del payload y deja `purgado: true`—, y después agrega un evento Purged con
// la cabeza que el registro tenía antes, qué eventos se tocaron y quién lo
// pidió.
//
// # ROMPE LA CADENA A PROPÓSITO
//
// El digest de un evento incluye su payload. Cambiarlo cambia el digest, y el
// evento siguiente guarda el digest viejo: la cadena se rompe ahí. Se podría
// re-encadenar todo lo que sigue y dejarla perfecta, y eso sería exactamente
// lo que el sello (§22b) existe para detectar: una historia rehecha que queda
// internamente consistente. Purgar no hace eso. Deja la rotura, y Verificar y
// CadenaHasta la explican con nombre y fecha en vez de decir "se rompió". Un
// sello publicado antes de la purga va a dejar de coincidir, y eso es
// correcto: la historia que se publicó ya no es la que hay.
const KindPurged = "Purged"

// Purga es el payload del evento Purged: lo que hace falta para explicar la
// rotura después.
type Purga struct {
	Nota           string   `json:"nota"`
	Eventos        []uint64 `json:"eventos"`         // los reescritos
	CabezaAnterior string   `json:"cabeza_anterior"` // digest de la cabeza antes de tocar nada
	SeqAnterior    uint64   `json:"seq_anterior"`
	Motivo         string   `json:"motivo,omitempty"`
}

// Purgar borra stdout y stderr de todos los eventos de la nota que los tengan,
// reescribiendo los archivos, y asienta un evento Purged. Devuelve qué se tocó
// y el evento asentado. Si ningún evento tenía salida, no reescribe nada y aun
// así asienta el Purged: que quede que alguien lo pidió.
//
// El Purged se encadena con la cabeza ANTERIOR a la reescritura. Si el evento
// purgado era el último, ningún otro guardaba su digest viejo y la cadena
// habría quedado perfecta —una purga invisible—; con el prev viejo en el
// Purged, la rotura queda ahí y se explica igual que las demás.
func (j *Journal) Purgar(id, quien, motivo string) (Purga, Event, error) {
	if strings.TrimSpace(id) == "" {
		return Purga{}, Event{}, fmt.Errorf("journal: purgar necesita el id de la nota")
	}
	if strings.TrimSpace(quien) == "" {
		quien = "human"
	}
	p, err := j.reescribirSinSalida(id)
	if err != nil {
		return Purga{}, Event{}, err
	}
	p.Motivo = strings.TrimSpace(motivo)
	payload, err := json.Marshal(p)
	if err != nil {
		return Purga{}, Event{}, err
	}
	ev, err := j.Append(Event{NoteID: id, Kind: KindPurged, Emitter: quien, Payload: payload, PrevDigest: p.CabezaAnterior})
	if err != nil {
		return p, Event{}, err
	}
	return p, ev, nil
}

// reescribirSinSalida hace la parte destructiva bajo los dos candados: el del
// proceso y el cerrojo del directorio, igual que una escritura.
func (j *Journal) reescribirSinSalida(id string) (Purga, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	c, err := bloquear(j.dir, esperaCerrojo)
	if err != nil {
		return Purga{}, err
	}
	defer c.liberar()
	j.ponerseAlDia()
	p := Purga{Nota: id, SeqAnterior: j.seq, CabezaAnterior: j.prev}

	files, err := filepath.Glob(filepath.Join(j.dir, "*.jsonl"))
	if err != nil {
		return Purga{}, err
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return Purga{}, err
		}
		lineas := strings.Split(string(b), "\n")
		cambiado := false
		for i, ln := range lineas {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			var e Event
			if json.Unmarshal([]byte(ln), &e) != nil {
				continue // una línea corrupta se deja como está
			}
			if e.NoteID != id || len(e.Payload) == 0 {
				continue
			}
			var carga map[string]any
			if json.Unmarshal(e.Payload, &carga) != nil {
				continue
			}
			_, hayOut := carga["stdout"]
			_, hayErr := carga["stderr"]
			if !hayOut && !hayErr {
				continue
			}
			delete(carga, "stdout")
			delete(carga, "stderr")
			carga["purgado"] = true
			e.Payload, err = json.Marshal(carga)
			if err != nil {
				return Purga{}, err
			}
			nl, err := json.Marshal(e)
			if err != nil {
				return Purga{}, err
			}
			lineas[i] = string(nl)
			cambiado = true
			p.Eventos = append(p.Eventos, e.Seq)
		}
		if cambiado {
			if err := atomico.Escribir(f, []byte(strings.Join(lineas, "\n")), 0o644); err != nil {
				return Purga{}, err
			}
		}
	}
	// El caché memorizó el registro de antes: que relea.
	j.muCache.Lock()
	j.cacheHuella = ""
	j.muCache.Unlock()
	// Y la punta en memoria puede haber cambiado de digest si el último evento
	// fue uno de los reescritos: Append la vuelve a leer del disco, pero
	// dejarla bien acá evita que Cabeza mienta mientras tanto.
	if len(p.Eventos) > 0 {
		j.cacheEvs = nil
		if evs, err := j.All(); err == nil && len(evs) > 0 {
			j.prev = evs[len(evs)-1].digest()
		}
	}
	return p, nil
}

// explicarRotura dice, si puede, POR QUÉ la cadena no cierra en el evento i:
// si el evento anterior fue reescrito por una purga posterior, la nombra con
// su fecha. Devuelve "" cuando no hay purga que lo explique — y entonces la
// rotura es lo que parece: alguien editó el registro.
func explicarRotura(evs []Event, i int) string {
	if i <= 0 {
		return ""
	}
	sospechoso := evs[i-1].Seq
	for _, e := range evs[i:] {
		if e.Kind != KindPurged {
			continue
		}
		var p Purga
		if json.Unmarshal(e.Payload, &p) != nil {
			continue
		}
		for _, s := range p.Eventos {
			if s == sospechoso {
				return fmt.Sprintf(" — porque la nota %q se purgó el %s (evento %d, pedido por %s): "+
					"se borró la salida del runner de %d evento(s) y la cadena se rompió a propósito",
					p.Nota, e.TxTime.UTC().Format("2006-01-02"), e.Seq, e.Emitter, len(p.Eventos))
			}
		}
	}
	return ""
}

// RotaPorPurga dice si el error de Verificar o CadenaHasta es una rotura que
// una purga explica. Quien lo muestra puede entonces decir "hubo una purga"
// en vez de "alguien editó el registro": la primera es una operación con
// dueño y fecha; la segunda, una acusación.
func RotaPorPurga(err error) bool {
	return err != nil && strings.Contains(err.Error(), "se purgó el")
}

// Purgas lista los eventos Purged, en orden. Es lo que el visor y `sellos`
// muestran al lado de una cadena rota: no "se rompió", sino "se purgó esto".
func (j *Journal) Purgas() ([]Purga, []Event, error) {
	evs, err := j.All()
	if err != nil {
		return nil, nil, err
	}
	var ps []Purga
	var out []Event
	for _, e := range evs {
		if e.Kind != KindPurged {
			continue
		}
		var p Purga
		if json.Unmarshal(e.Payload, &p) != nil {
			continue
		}
		ps = append(ps, p)
		out = append(out, e)
	}
	return ps, out, nil
}
