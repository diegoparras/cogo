// Package recibo guarda, por cada `authorize`, qué sabía el agente cuando pidió.
//
// # POR QUÉ
//
// El registro de eventos es bitemporal: se puede reconstruir el estado del
// vault en cualquier instante. Nadie usaba eso. Con el recibo, "¿qué sabía el
// agente a las 15:32 cuando borró la tabla?" tiene respuesta exacta: qué
// pidió, quién, qué notas citó y en qué estado estaban, y hasta qué evento
// del registro llegaba la historia en ese momento.
//
// # QUÉ PRUEBA
//
// El recibo lleva la CABEZA del registro en el momento del veredicto: el
// número y el digest del último evento. Reconstruir compara ese digest con el
// que el registro de hoy calcula para ese mismo número. Si coinciden, toda la
// historia hasta ahí es la misma que entonces, y los estados del recibo se
// pueden volver a derivar. Si no coinciden, alguien reescribió el registro
// después del veredicto — y eso también es una respuesta.
//
// Lo que NO prueba: que el archivo de la nota fuera el mismo. El estado final
// depende de la evidencia y la frescura, que se leen del archivo en cada
// evaluación. Por eso el recibo guarda dos estados por nota: el del eje de
// eventos (reconstruible con exactitud) y el final (el que decidió).
package recibo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/journal"
)

// Nota es una nota tal como estaba cuando se pidió.
type Nota struct {
	ID string `json:"id"`
	// Final es el estado que decidió (con evidencia, frescura, propagación).
	Final string `json:"final"`
	// Eventos es el estado del eje de eventos, plegado hasta Seq: lo único que
	// se puede reconstruir con exactitud.
	Eventos string `json:"eventos"`
}

// Recibo es un veredicto de authorize con su contexto.
type Recibo struct {
	ID       string    `json:"id"`
	Cuando   time.Time `json:"cuando"`
	Quien    string    `json:"quien"`
	Accion   string    `json:"accion"`
	Clase    string    `json:"clase"`
	Necesita string    `json:"necesita"`
	Autoriza bool      `json:"autoriza"`
	Porque   string    `json:"porque,omitempty"`
	Bloqueo  string    `json:"bloqueo,omitempty"`
	Notas    []Nota    `json:"notas,omitempty"`
	// Descartadas son las que Jev sacó por no hablar de la acción.
	Descartadas []string `json:"descartadas,omitempty"`
	// Juicios son los números de Jev que entraron, por nota citada.
	Juicios map[string]float64 `json:"juicios,omitempty"`
	// Seq y Cabeza: hasta dónde llegaba el registro. Es lo que hace al recibo
	// verificable después.
	Seq    uint64 `json:"seq"`
	Cabeza string `json:"cabeza"`
}

// Store es el archivo append-only de recibos.
type Store struct {
	ruta string
	mu   sync.Mutex
}

func Abrir(vault string) *Store {
	return &Store{ruta: filepath.Join(vault, ".cogo", "recibos.jsonl")}
}

// Agregar escribe un recibo y le da su id: el hash corto de su contenido más
// el instante. Dos recibos idénticos en el mismo nanosegundo no existen.
func (s *Store) Agregar(r Recibo) (Recibo, error) {
	if strings.TrimSpace(r.Accion) == "" {
		return r, fmt.Errorf("recibo: sin acción no hay nada que asentar")
	}
	if r.Cuando.IsZero() {
		r.Cuando = time.Now().UTC()
	}
	if r.ID == "" {
		h := sha256.New()
		fmt.Fprintf(h, "%s|%s|%s|%d|%s", r.Cuando.Format(time.RFC3339Nano), r.Quien, r.Accion, r.Seq, r.Cabeza)
		r.ID = "r-" + r.Cuando.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(h.Sum(nil))[:6]
	}
	b, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.ruta), 0o755); err != nil {
		return r, err
	}
	f, err := os.OpenFile(s.ruta, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return r, err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return r, err
}

// Todos devuelve los recibos, el más reciente primero.
func (s *Store) Todos() ([]Recibo, error) {
	s.mu.Lock()
	b, err := os.ReadFile(s.ruta)
	s.mu.Unlock()
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Recibo
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var r Recibo
		if json.Unmarshal([]byte(ln), &r) == nil {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Cuando.After(out[j].Cuando) })
	return out, nil
}

// Buscar devuelve un recibo por id.
func (s *Store) Buscar(id string) (Recibo, bool) {
	todos, err := s.Todos()
	if err != nil {
		return Recibo{}, false
	}
	for _, r := range todos {
		if r.ID == id {
			return r, true
		}
	}
	return Recibo{}, false
}

// EstadosDeEventos pliega el registro hasta `seq` y devuelve el estado del eje
// de eventos de cada nota citada. Es la mitad del recibo que se reconstruye.
func EstadosDeEventos(evs []journal.Event, seq uint64, ids []string) map[string]string {
	var hasta []journal.Event
	for _, e := range evs {
		if e.Seq <= seq {
			hasta = append(hasta, e)
		}
	}
	plegado := journal.Fold(hasta, time.Time{}, time.Time{})
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		est, ok := plegado[id]
		if !ok {
			est = confidence.Inicial
		}
		out[id] = est.String()
	}
	return out
}

// Reconstruccion es lo que hoy se puede decir de un recibo de entonces.
type Reconstruccion struct {
	Fiel   bool   `json:"fiel"`
	Motivo string `json:"motivo"`
	// Eventos es el estado del eje de eventos de cada nota citada, plegado
	// HOY hasta el Seq del recibo. Si el recibo es fiel, coincide.
	Eventos map[string]string `json:"eventos,omitempty"`
}

// Reconstruir comprueba un recibo contra el registro de hoy.
//
// Dos cosas, en orden: que el digest del evento Seq sea el que el recibo
// guardó (la historia hasta ahí no se reescribió), y que el pliegue hasta Seq
// dé los mismos estados de eventos. La primera ya implica la segunda; la
// segunda existe para NOMBRAR qué nota cambió cuando la primera falla.
func Reconstruir(j *journal.Journal, r Recibo) Reconstruccion {
	evs, err := j.All()
	if err != nil {
		return Reconstruccion{Motivo: "no se pudo leer el registro: " + err.Error()}
	}
	ids := make([]string, 0, len(r.Notas))
	for _, n := range r.Notas {
		ids = append(ids, n.ID)
	}
	hoy := EstadosDeEventos(evs, r.Seq, ids)
	out := Reconstruccion{Eventos: hoy}
	if r.Seq == 0 && r.Cabeza == "" {
		out.Fiel, out.Motivo = true, "el registro estaba vacío cuando se pidió; no hay historia que comparar"
		return out
	}
	// La cadena se REHACE desde el principio: un evento anterior editado cambia
	// el digest que sale acá aunque el archivo conserve los prev de entonces.
	digest, err := j.CadenaHasta(r.Seq)
	switch {
	case err != nil && strings.Contains(err.Error(), "no llega"):
		out.Motivo = fmt.Sprintf("el registro de hoy no llega al evento %d: se recortó o se reescribió después del veredicto", r.Seq)
		return out
	case err != nil:
		out.Motivo = "la historia se reescribió después del veredicto: " + err.Error()
		return out
	case digest != r.Cabeza:
		out.Motivo = fmt.Sprintf("el evento %d tiene hoy otro digest: la historia se reescribió después del veredicto", r.Seq)
		for _, n := range r.Notas {
			if hoy[n.ID] != n.Eventos {
				out.Motivo += fmt.Sprintf("; %s estaba en %s y hoy se pliega a %s", n.ID, n.Eventos, hoy[n.ID])
			}
		}
		return out
	}
	for _, n := range r.Notas {
		if hoy[n.ID] != n.Eventos {
			out.Motivo = fmt.Sprintf("la cabeza coincide pero %s se pliega distinto (%s → %s): el registro tiene eventos con número anterior insertados después", n.ID, n.Eventos, hoy[n.ID])
			return out
		}
	}
	out.Fiel, out.Motivo = true, "la historia hasta ese evento es la misma que entonces"
	return out
}
