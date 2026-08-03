// Package journal es el registro append-only de todo lo que le pasó a una nota.
//
// Existe para responder una pregunta que hoy COGO no puede contestar: qué creía
// el sistema en un momento dado. Es la diferencia entre guardar el estado actual
// y poder auditar una decisión que un agente tomó la semana pasada.
//
// Dos ejes de tiempo, como en el modelo bitemporal de SQL:2011:
//
//	ValidTime — cuándo pasó en el mundo
//	TxTime    — cuándo COGO se enteró
//
// Se distinguen porque no siempre coinciden: un archivo pudo cambiar el martes y
// COGO verlo el jueves. Para auditar "¿qué sabía el agente cuando actuó?" la
// pregunta es sobre el segundo eje; para "¿desde cuándo esto era falso?", sobre
// el primero.
//
// El archivo es JSONL, uno por mes, legible con cualquier editor. Cada evento
// encadena el hash del anterior, así que una manipulación posterior se nota sin
// necesidad de firmar nada.
package journal

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
)

// Event es una cosa que le pasó a una nota. El payload queda crudo para que
// agregar campos no obligue a migrar los eventos ya escritos.
type Event struct {
	Seq        uint64          `json:"seq"`        // monotónico por vault
	ValidTime  time.Time       `json:"valid_time"` // cuándo pasó en el mundo
	TxTime     time.Time       `json:"tx_time"`    // cuándo COGO lo registró
	NoteID     string          `json:"note_id"`
	Kind       string          `json:"kind"`            // el evento de la máquina de estados
	Emitter    string          `json:"emitter"`         // internal_runner | agent | human | guard | xray
	Guard      string          `json:"guard,omitempty"` // qué guarda se cumplió, si la transición discrimina
	Fence      uint64          `json:"fence,omitempty"` // token del lease vigente
	Payload    json.RawMessage `json:"payload,omitempty"`
	PrevDigest string          `json:"prev"` // hash del evento anterior
}

// digest encadena: el hash de este evento incluye el del anterior, así que
// alterar uno viejo invalida todos los que siguen.
func (e Event) digest() string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s|%s|%d|%s|%s",
		e.Seq, e.ValidTime.UTC().Format(time.RFC3339Nano), e.TxTime.UTC().Format(time.RFC3339Nano),
		e.NoteID, e.Kind, e.Emitter, e.Guard, e.Fence, string(e.Payload), e.PrevDigest)
	return hex.EncodeToString(h.Sum(nil))
}

// Journal es el registro de un vault.
type Journal struct {
	dir  string
	mu   sync.Mutex
	seq  uint64
	prev string
	// ahora se inyecta para que los tests no dependan del reloj.
	ahora func() time.Time
}

// Open abre (o crea) el journal de un vault y se pone al día con lo que ya hay
// escrito, para que el número de secuencia y la cadena continúen.
func Open(vault string) (*Journal, error) {
	dir := filepath.Join(vault, ".cogo", "journal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	j := &Journal{dir: dir, ahora: time.Now}
	evs, err := j.All()
	if err != nil {
		return nil, err
	}
	if n := len(evs); n > 0 {
		j.seq = evs[n-1].Seq
		j.prev = evs[n-1].digest()
	}
	return j, nil
}

// SetClock inyecta el reloj. Solo para tests.
func (j *Journal) SetClock(f func() time.Time) { j.mu.Lock(); j.ahora = f; j.mu.Unlock() }

// Append escribe un evento. Completa seq, tx_time y el encadenado; si valid_time
// viene vacío, se asume que pasó cuando se registró.
func (j *Journal) Append(e Event) (Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := j.ahora().UTC()
	e.TxTime = now
	if e.ValidTime.IsZero() {
		e.ValidTime = now
	}
	// El tiempo de validez puede ser anterior al de registro (algo que se supo
	// después), pero nunca posterior: eso sería registrar el futuro.
	if e.ValidTime.After(now) {
		return Event{}, fmt.Errorf("journal: valid_time %s es posterior a tx_time %s", e.ValidTime, now)
	}
	if strings.TrimSpace(e.NoteID) == "" || strings.TrimSpace(e.Kind) == "" {
		return Event{}, fmt.Errorf("journal: un evento necesita note_id y kind")
	}
	j.seq++
	e.Seq = j.seq
	e.PrevDigest = j.prev

	line, err := json.Marshal(e)
	if err != nil {
		j.seq-- // no quedó escrito: devolver el número
		return Event{}, err
	}
	path := filepath.Join(j.dir, now.Format("2006-01")+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		j.seq--
		return Event{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		j.seq--
		return Event{}, err
	}
	j.prev = e.digest()
	return e, nil
}

// All devuelve todos los eventos, ordenados por número de secuencia. Los
// archivos son por mes y se leen en orden, así que la cadena se reconstruye
// aunque haya rotado.
func (j *Journal) All() ([]Event, error) {
	files, err := filepath.Glob(filepath.Join(j.dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files) // "2026-01" < "2026-02": el nombre ya ordena
	var out []Event
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			var e Event
			if err := json.Unmarshal([]byte(ln), &e); err != nil {
				// Una línea corrupta no debe hacer ilegible el resto del
				// registro: se saltea y se sigue.
				continue
			}
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, k int) bool { return out[i].Seq < out[k].Seq })
	return out, nil
}

// Desde devuelve los eventos posteriores a un número de secuencia. Es lo que
// convierte al journal en un cursor: un agente pregunta "qué cambió desde que
// miré" en vez de releer todo.
func (j *Journal) Desde(seq uint64) ([]Event, error) {
	all, err := j.All()
	if err != nil {
		return nil, err
	}
	i := sort.Search(len(all), func(i int) bool { return all[i].Seq > seq })
	return all[i:], nil
}

// Verificar recorre la cadena y devuelve el primer eslabón roto. Un journal
// íntegro devuelve nil: cada evento encadena el hash del anterior, así que
// editar uno viejo invalida todos los que siguen y esto lo encuentra.
func (j *Journal) Verificar() error {
	evs, err := j.All()
	if err != nil {
		return err
	}
	prev := ""
	for _, e := range evs {
		if e.PrevDigest != prev {
			return fmt.Errorf("journal: la cadena se rompe en el evento %d (nota %q): esperaba prev=%q y tiene %q",
				e.Seq, e.NoteID, prev, e.PrevDigest)
		}
		prev = e.digest()
	}
	return nil
}

// Seq es el último número de secuencia escrito.
func (j *Journal) Seq() uint64 { j.mu.Lock(); defer j.mu.Unlock(); return j.seq }
