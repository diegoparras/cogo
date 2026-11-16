package jev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/atomico"
)

// Registro guarda dos cosas de cada juicio.
//
// El CACHÉ (.cogo/jev-cache.json): la respuesta por clave de pregunta, para
// que el mismo juicio no se pague dos veces. Es lo que hace posible que los
// techos —nivel de evidencia, pertinencia del check— se consulten en cada
// evaluación sin una llamada HTTP por nota: la evaluación lee el caché y nada
// más; quien lo llena es Precalentar, por lotes, antes.
//
// El CRUDO (.cogo/jev.jsonl): cada respuesta tal cual vino, con el modelo y la
// latencia. Es lo que permite mover un umbral mañana sin volver a preguntar, y
// es lo que va al recibo cuando exista.
type Registro struct {
	dir     string
	mu      sync.Mutex
	cache   map[string]Respuesta
	cargado bool
	sucio   bool
}

func abrirRegistro(dir string) *Registro {
	return &Registro{dir: dir, cache: map[string]Respuesta{}}
}

func (r *Registro) rutaCache() string { return filepath.Join(r.dir, ".cogo", "jev-cache.json") }
func (r *Registro) rutaCrudo() string { return filepath.Join(r.dir, ".cogo", "jev.jsonl") }

// clave identifica una pregunta por su tipo y sus entradas. El hash es
// deliberado: el estado puede llevar un comando o un claim, y el archivo de
// caché no tiene por qué repetirlos.
func clave(tipo string, partes ...string) string {
	h := sha256.New()
	h.Write([]byte(tipo))
	for _, p := range partes {
		h.Write([]byte{0})
		h.Write([]byte(strings.TrimSpace(p)))
	}
	return tipo + ":" + hex.EncodeToString(h.Sum(nil))[:24]
}

func (r *Registro) cargar() {
	if r.cargado {
		return
	}
	r.cargado = true
	b, err := os.ReadFile(r.rutaCache())
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &r.cache)
	if r.cache == nil {
		r.cache = map[string]Respuesta{}
	}
}

// Get devuelve la respuesta guardada para esa clave.
func (r *Registro) Get(k string) (Respuesta, bool) {
	if r == nil {
		return Respuesta{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cargar()
	v, ok := r.cache[k]
	return v, ok
}

// Put guarda una respuesta y deja la línea cruda.
func (r *Registro) Put(k, tipo string, resp Respuesta, modelo string, dur time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cargar()
	r.cache[k] = resp
	r.sucio = true
	linea, _ := json.Marshal(map[string]any{
		"t": time.Now().UTC().Format(time.RFC3339), "tipo": tipo, "clave": k,
		"respuesta": resp, "modelo": modelo, "ms": dur.Milliseconds(),
	})
	_ = os.MkdirAll(filepath.Dir(r.rutaCrudo()), 0o755)
	if f, err := os.OpenFile(r.rutaCrudo(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		_, _ = f.Write(append(linea, '\n'))
		_ = f.Close()
	}
}

// Guardar persiste el caché si cambió. Se llama al final de cada lote, no en
// cada Put: cien juicios son cien líneas crudas y UNA escritura del caché.
func (r *Registro) Guardar() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sucio {
		return
	}
	b, err := json.MarshalIndent(r.cache, "", " ")
	if err != nil {
		return
	}
	if atomico.Escribir(r.rutaCache(), b, 0o644) == nil {
		r.sucio = false
	}
}
