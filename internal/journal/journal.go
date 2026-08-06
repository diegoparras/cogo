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

	// La lectura completa, memorizada. Ver All: leer un journal es leer y
	// parsear TODO, y hay varios llamadores que lo necesitan entero en la misma
	// petición. Va con su propio candado para no cruzarse con el de escritura.
	muCache     sync.Mutex
	cacheEvs    []Event
	cacheHuella string
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

// EmisorEjecucion es el emisor reservado: el único cuyos eventos pueden llevar
// una nota a `verified`. Está acá y no en el runner para que el journal pueda
// defenderlo.
const EmisorEjecucion = "internal_runner"

// ErrEmisorReservado se devuelve cuando alguien intenta emitir con el emisor
// privilegiado por la puerta común.
var ErrEmisorReservado = fmt.Errorf("journal: %q es el emisor reservado del runner; usá AppendEjecucion", EmisorEjecucion)

// Append escribe un evento.
//
// Rechaza el emisor reservado. Go no tiene paquetes "amigos", así que no se
// puede impedir por tipos que otro paquete escriba esa cadena — pero sí se puede
// obligar a que pase por una puerta con nombre propio, que además hace que un
// grep de AppendEjecucion muestre TODOS los lugares que producen verificaciones.
// Una cadena mágica esparcida no se audita; una función sí.
func (j *Journal) Append(e Event) (Event, error) {
	if e.Emitter == EmisorEjecucion {
		return Event{}, ErrEmisorReservado
	}
	return j.escribir(e)
}

// AppendEjecucion es la única puerta del emisor reservado. La usa el runner
// cuando ejecutó un check de verdad y observó su código de salida.
func (j *Journal) AppendEjecucion(e Event) (Event, error) {
	e.Emitter = EmisorEjecucion
	return j.escribir(e)
}

// escribir completa seq, tx_time y el encadenado; si valid_time viene vacío, se
// asume que pasó cuando se registró.
func (j *Journal) escribir(e Event) (Event, error) {
	// El orden importa: primero el candado del proceso, que serializa las
	// goroutines, y recién después el cerrojo del sistema, que serializa los
	// procesos. Al revés, cada goroutine competiría por el cerrojo del sistema
	// contra las suyas propias, que es caro y no hace falta.
	j.mu.Lock()
	defer j.mu.Unlock()

	c, err := bloquear(j.dir, esperaCerrojo)
	if err != nil {
		return Event{}, err
	}
	defer c.liberar()

	// Con el cerrojo en la mano, la punta del registro se relee del disco. El
	// número de secuencia que este proceso tiene en memoria puede haber quedado
	// viejo si otro escribió mientras tanto, y escribir sobre un número ya usado
	// es lo único que este cerrojo existe para impedir.
	j.ponerseAlDia()

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
	j.extenderCache(e)
	return e, nil
}

// extenderCache agrega el evento recién escrito a la lectura memorizada, en vez
// de invalidarla. Invalidar sería correcto y costaría una relectura completa en
// la siguiente evaluación — justo después de capturar una nota, que es cuando
// alguien está mirando.
//
// El append es directo, sin copiar: lo que se entrega a los llamadores es una
// VISTA recortada (ver vista), así que la capacidad de sobra que usa este append
// queda fuera de lo que cualquiera de ellos puede ver o tocar.
//
// La versión anterior copiaba el registro entero en cada escritura para poder
// entregarlo directo. Era correcta, y era O(n) por evento: sobre un registro de
// veinte mil, cada captura movía cuatro megabytes.
func (j *Journal) extenderCache(e Event) {
	h, err := j.huella()
	j.muCache.Lock()
	defer j.muCache.Unlock()
	if err != nil || h == "" || j.cacheHuella == "" {
		j.cacheHuella = "" // no se pudo confirmar, o no había nada: que relea
		return
	}

	// El número de secuencia dice exactamente dónde está parado el caché, y hace
	// falta preguntárselo: entre que el evento se escribió al archivo y que llega
	// acá, otra goroutine pudo haber leído el disco y ya tenerlo. Agregarlo
	// entonces lo duplicaría, y un registro con un evento repetido rompe la
	// verificación de la cadena.
	var ultimo uint64
	if n := len(j.cacheEvs); n > 0 {
		ultimo = j.cacheEvs[n-1].Seq
	}
	switch {
	case ultimo == e.Seq:
		j.cacheHuella = h // ya estaba: solo hay que ponerlo al día
	case ultimo == e.Seq-1:
		j.cacheEvs = append(j.cacheEvs, e)
		j.cacheHuella = h
	default:
		j.cacheHuella = "" // desfasado por algo que no fue esta escritura: que relea
	}
}

// huella identifica el contenido del directorio sin leerlo: nombre, tamaño y
// fecha de cada archivo. Un journal es append-only, así que cualquier evento
// nuevo —lo escriba este proceso u otro— cambia el tamaño de algún archivo.
//
// Devuelve "" si algo no se pudo consultar. Una huella vacía nunca coincide, y
// esa es la respuesta correcta ante la duda: releer.
func (j *Journal) huella() (string, error) {
	files, err := filepath.Glob(filepath.Join(j.dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	var b strings.Builder
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			return "", nil
		}
		fmt.Fprintf(&b, "%s|%d|%d\n", filepath.Base(f), st.Size(), st.ModTime().UnixNano())
	}
	return b.String(), nil
}

// ponerseAlDia adopta la punta que está en el disco si va más adelante que la
// que este proceso recuerda. Se apoya en el caché de All: cuando nadie más
// escribió, la huella coincide y no se lee nada.
//
// Solo adopta hacia ADELANTE. Un disco que quedó ATRÁS de la memoria no es otro
// proceso escribiendo: es un archivo truncado o restaurado por atrás, y ahí
// reescribir números ya usados empeoraría las cosas. Se sigue de largo y que lo
// denuncie Verificar.
func (j *Journal) ponerseAlDia() {
	evs, err := j.All()
	if err != nil || len(evs) == 0 {
		return
	}
	u := evs[len(evs)-1]
	if u.Seq >= j.seq {
		j.seq = u.Seq
		j.prev = u.digest()
	}
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

	h, err := j.huella()
	if err != nil {
		return nil, err
	}
	j.muCache.Lock()
	defer j.muCache.Unlock()
	if h != "" && h == j.cacheHuella {
		return j.vista(), nil
	}

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
	j.cacheEvs = out
	j.cacheHuella = h
	return j.vista(), nil
}

// vista es lo que se le entrega a un llamador: el registro memorizado recortado
// a su largo exacto.
//
// El recorte es lo que hace seguro compartirlo. Un slice con capacidad de sobra
// deja que quien le haga append escriba sobre el array que están leyendo los
// demás; uno donde largo y capacidad coinciden obliga a Go a reservar uno nuevo.
// Y no cuesta nada: es un encabezado de slice, no una copia.
func (j *Journal) vista() []Event {
	return j.cacheEvs[:len(j.cacheEvs):len(j.cacheEvs)]
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
	var anterior uint64
	for i, e := range evs {
		// Los números repetidos o que retroceden tienen una causa conocida —dos
		// procesos escribiendo el mismo registro— y merecen decirlo con nombre.
		// Un "la cadena se rompe" a secas manda a buscar el problema al lugar
		// equivocado.
		if i > 0 && e.Seq <= anterior {
			return fmt.Errorf("journal: el evento %d (nota %q) repite o retrocede el número de secuencia %d. "+
				"Es lo que pasa cuando dos procesos escriben el mismo vault sin coordinarse",
				e.Seq, e.NoteID, anterior)
		}
		anterior = e.Seq
		if e.PrevDigest != prev {
			return fmt.Errorf("journal: la cadena se rompe en el evento %d (nota %q): esperaba prev=%q y tiene %q",
				e.Seq, e.NoteID, prev, e.PrevDigest)
		}
		prev = e.digest()
	}
	return nil
}

// Cabeza es el estado del registro en un solo par de valores: hasta qué número
// de secuencia llegó, y el digest de ese último evento.
//
// Ese digest resume TODA la historia anterior, porque el hash de cada evento
// incluye el del anterior. Es lo único que hace falta publicar afuera para que
// después se pueda probar que el registro no se reescribió: un hash de 64
// caracteres contra el que se compara la cadena entera.
func (j *Journal) Cabeza() (uint64, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.seq, j.prev
}

// DigestDe recalcula el digest del evento con ese número de secuencia, leyendo
// del disco. Es lo que permite comprobar un sello viejo: si alguien reescribió
// el registro, el digest que sale de los eventos de hoy no va a coincidir con el
// que se publicó entonces.
func (j *Journal) DigestDe(seq uint64) (string, bool) {
	evs, err := j.All()
	if err != nil {
		return "", false
	}
	for _, e := range evs {
		if e.Seq == seq {
			return e.digest(), true
		}
	}
	return "", false
}

// Seq es el último número de secuencia escrito.
func (j *Journal) Seq() uint64 { j.mu.Lock(); defer j.mu.Unlock(); return j.seq }
