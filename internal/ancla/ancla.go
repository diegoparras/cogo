// Package ancla publica la cabeza del registro afuera, para poder probar
// después que no se reescribió.
//
// # QUÉ TAPA
//
// El registro de eventos está encadenado por hash: alterar un evento viejo
// invalida todos los que siguen, y Verificar lo detecta. Eso alcanza contra la
// corrupción y contra un editor descuidado.
//
// No alcanza contra el DUEÑO DEL VAULT. Quien tiene el archivo puede regenerar
// el registro entero desde cero —eventos nuevos, hashes recalculados, cadena
// perfecta— y no hay forma de notarlo, porque no existe ningún punto de
// referencia fuera de su disco. La cadena prueba consistencia interna, no
// historia.
//
// # LA SOLUCIÓN, Y POR QUÉ NO ES UNA BLOCKCHAIN
//
// Alcanza con publicar UN hash —la cabeza de la cadena— en algún lugar que el
// dueño no controle. Ese hash resume toda la historia anterior, así que si el
// registro se reescribe, la cabeza que sale de los eventos de hoy no coincide
// con la que se publicó entonces.
//
// Una blockchain daría lo mismo más consenso distribuido, que resuelve el
// problema de ordenar escrituras entre partes que desconfían entre sí. COGO no
// tiene esa forma: hay UN escritor por vault, forzado por un cerrojo del sistema
// operativo. Comprar consenso para un solo escritor es pagar por un problema
// que no se tiene — y encima obligaría a que COGO dependa de una red, cuando
// hoy todo lo que toca la red es un accesorio opcional.
//
// # LO QUE UN SELLO PRUEBA Y LO QUE NO
//
// Prueba que en el momento en que se publicó, el registro era exactamente el que
// produce esa cabeza. Si el sello se publicó en un lugar con fecha propia
// (OpenTimestamps, un log de transparencia, un commit firmado), prueba además
// CUÁNDO.
//
// No prueba que lo que dicen los eventos sea cierto. Un sello es sobre la
// historia, no sobre los hechos.
//
// Y un sello que solo vive en el disco de uno no prueba nada contra uno mismo:
// el valor entero está en dónde se publicó. Por eso el archivo local guarda
// SIEMPRE dónde fue y qué contestó el destino.
package ancla

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sello es una cabeza de la cadena, publicada afuera.
type Sello struct {
	Cuando time.Time `json:"cuando"`
	Seq    uint64    `json:"seq"`
	// Cabeza es el digest del evento Seq: el resumen de toda la historia hasta
	// ahí. Es lo único que se publica.
	Cabeza string `json:"cabeza"`
	// Donde identifica al destino ("manual", "https://…"). Sin esto el sello no
	// vale nada: un hash guardado en el mismo disco que el registro no prueba
	// nada contra quien tiene los dos.
	Donde string `json:"donde"`
	// Recibo es lo que contestó el destino — un id, una URL, una prueba. Es lo
	// que después permite ir a buscarlo.
	Recibo string `json:"recibo,omitempty"`
	// Nota la escribe una persona cuando publica a mano: "commit a1b2c3",
	// "posteado en X", lo que sea que permita encontrarlo de nuevo.
	Nota string `json:"nota,omitempty"`
}

// Store son los sellos de un vault: un archivo append-only, en texto.
type Store struct {
	ruta string
	mu   sync.Mutex
}

func Abrir(vault string) *Store {
	return &Store{ruta: filepath.Join(vault, ".cogo", "sellos.jsonl")}
}

// Agregar registra un sello nuevo.
func (s *Store) Agregar(x Sello) error {
	if strings.TrimSpace(x.Cabeza) == "" {
		return fmt.Errorf("ancla: un sello sin cabeza no sella nada")
	}
	if strings.TrimSpace(x.Donde) == "" {
		return fmt.Errorf("ancla: un sello tiene que decir dónde se publicó, " +
			"o es un hash guardado al lado del registro que resume")
	}
	if x.Cuando.IsZero() {
		x.Cuando = time.Now().UTC()
	}
	b, err := json.Marshal(x)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.ruta), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.ruta, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Todos devuelve los sellos, del más nuevo al más viejo.
func (s *Store) Todos() ([]Sello, error) {
	s.mu.Lock()
	b, err := os.ReadFile(s.ruta)
	s.mu.Unlock()
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Sello
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var x Sello
		if json.Unmarshal([]byte(ln), &x) == nil {
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq > out[j].Seq })
	return out, nil
}

// Ultimo es el sello más reciente.
func (s *Store) Ultimo() (Sello, bool) {
	xs, err := s.Todos()
	if err != nil || len(xs) == 0 {
		return Sello{}, false
	}
	return xs[0], true
}

// Resultado es el veredicto sobre un sello.
type Resultado struct {
	Sello Sello  `json:"sello"`
	OK    bool   `json:"ok"`
	Dice  string `json:"dice"`
}

// Verificar recalcula la cabeza de cada sello contra el registro de HOY.
//
// digestDe lo provee el journal. Se pasa como función para que este paquete no
// dependa del registro: lo único que necesita saber es cómo pedir el digest de
// un número de secuencia.
//
// Un sello que no coincide es la señal que todo esto existe para dar: el
// registro que hay ahora no es el que se publicó entonces.
func Verificar(sellos []Sello, digestDe func(seq uint64) (string, bool)) []Resultado {
	out := make([]Resultado, 0, len(sellos))
	for _, x := range sellos {
		d, hay := digestDe(x.Seq)
		switch {
		case !hay:
			out = append(out, Resultado{Sello: x, Dice: fmt.Sprintf(
				"el registro de hoy no llega al evento %d: o se truncó, o este sello es de otro vault", x.Seq)})
		case d != x.Cabeza:
			out = append(out, Resultado{Sello: x, Dice: fmt.Sprintf(
				"NO COINCIDE — el evento %d sellado el %s tenía la cabeza %s y hoy da %s. "+
					"El registro se reescribió después de publicar este sello",
				x.Seq, x.Cuando.UTC().Format("2006-01-02"), corto(x.Cabeza), corto(d))})
		default:
			out = append(out, Resultado{Sello: x, OK: true, Dice: fmt.Sprintf(
				"coincide con lo publicado el %s en %s", x.Cuando.UTC().Format("2006-01-02"), x.Donde)})
		}
	}
	return out
}

func corto(h string) string {
	if len(h) > 12 {
		return h[:12] + "…"
	}
	return h
}
