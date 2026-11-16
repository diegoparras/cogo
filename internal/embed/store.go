package embed

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// El caché de embeddings se guarda en DOS archivos, y la división es deliberada:
//
//	.cogo/embeddings.meta.json   legible: modelo, dimensiones, y para cada nota su
//	                             hash de contenido y su posición. Es lo único que
//	                             un humano querría inspeccionar o diffear.
//	.cogo/embeddings.bin         los vectores crudos (float32). Mil quinientos
//	                             números por nota no le dicen nada a nadie; en
//	                             JSON ocupaban ~13 bytes por número y había que
//	                             parsearlos ENTEROS en cada búsqueda.
//
// Lo que se pierde al pasar a binario no es información sino ruido: los floats no
// eran inspeccionables en ningún sentido útil. Lo que importa —qué notas están
// embebidas, con qué modelo y si su hash sigue vigente— queda en texto plano.
//
// Y sigue siendo un CACHÉ, no un dato: se puede borrar entero y COGO lo reconstruye
// (cuesta llamadas al modelo, no conocimiento). Por eso `/api/export` nunca lo
// incluyó: el respaldo son tus notas.

const binVersion = 1

type slot struct {
	Hash string `json:"h"` // hash del texto embebido: si cambia, hay que re-embeber
	Off  int    `json:"i"` // índice del vector dentro del .bin
}

type meta struct {
	Version int             `json:"version"`
	Dims    int             `json:"dims"`
	Notes   map[string]slot `json:"notes"`
}

func metaPath(dir string) string { return filepath.Join(dir, ".cogo", "embeddings.meta.json") }
func binPath(dir string) string  { return filepath.Join(dir, ".cogo", "embeddings.bin") }
func legacyPath(dir string) string {
	return filepath.Join(dir, ".cogo", "embeddings.json")
}

// store es el caché ya cargado en memoria.
type store struct {
	dims int
	vec  map[string][]float32 // id -> vector
	hash map[string]string    // id -> hash del texto
}

func newStore() *store {
	return &store{vec: map[string][]float32{}, hash: map[string]string{}}
}

// Un proceso largo (el servidor MCP) atiende muchas búsquedas: leer y parsear el
// caché en cada una era el verdadero cuello de botella. Se carga una vez y se
// reusa mientras el archivo no cambie.
var (
	memMu sync.Mutex
	memBy = map[string]*cached{}
)

type cached struct {
	st   *store
	size int64
	mod  int64
}

// load devuelve el caché del vault, releyéndolo solo si cambió en disco.
func load(dir string) *store {
	memMu.Lock()
	defer memMu.Unlock()
	fi, err := os.Stat(metaPath(dir))
	if err == nil {
		if c, ok := memBy[dir]; ok && c.size == fi.Size() && c.mod == fi.ModTime().UnixNano() {
			return c.st
		}
	}
	st := readFrom(dir)
	if fi != nil && err == nil {
		memBy[dir] = &cached{st: st, size: fi.Size(), mod: fi.ModTime().UnixNano()}
	}
	return st
}

func readFrom(dir string) *store {
	st := newStore()
	mb, err := os.ReadFile(metaPath(dir))
	if err != nil {
		return migrateLegacy(dir) // primera vez tras actualizar: convertir el JSON viejo
	}
	var m meta
	if json.Unmarshal(mb, &m) != nil || m.Dims <= 0 {
		return st
	}
	raw, err := os.ReadFile(binPath(dir))
	if err != nil {
		return st
	}
	st.dims = m.Dims
	for id, s := range m.Notes {
		start := s.Off * m.Dims * 4
		if start < 0 || start+m.Dims*4 > len(raw) {
			continue // meta y bin desalineados: se ignora y se re-embebe
		}
		v := make([]float32, m.Dims)
		for i := 0; i < m.Dims; i++ {
			v[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[start+i*4:]))
		}
		st.vec[id], st.hash[id] = v, s.Hash
	}
	return st
}

// migrateLegacy convierte el `.cogo/embeddings.json` de la versión anterior. Se
// hace en silencio y una sola vez: re-embeber cuesta llamadas al modelo (plata),
// así que descartar el caché viejo sería cobrarle al usuario una decisión nuestra.
func migrateLegacy(dir string) *store {
	st := newStore()
	b, err := os.ReadFile(legacyPath(dir))
	if err != nil {
		return st
	}
	var old map[string]struct {
		Hash string    `json:"h"`
		Vec  []float32 `json:"v"`
	}
	if json.Unmarshal(b, &old) != nil {
		return st
	}
	for id, e := range old {
		if len(e.Vec) == 0 {
			continue
		}
		if st.dims == 0 {
			st.dims = len(e.Vec)
		}
		if len(e.Vec) != st.dims {
			continue
		}
		st.vec[id], st.hash[id] = e.Vec, e.Hash
	}
	if len(st.vec) > 0 && save(dir, st) == nil {
		_ = os.Remove(legacyPath(dir))
	}
	return st
}

// save reescribe los dos archivos. Se compacta entero en vez de llevar huecos y
// listas libres: pasa solo cuando algo cambió, y la simplicidad vale más que
// ahorrar una escritura de unos pocos megas.
func save(dir string, st *store) error {
	if st.dims == 0 {
		return nil
	}
	m := meta{Version: binVersion, Dims: st.dims, Notes: make(map[string]slot, len(st.vec))}
	raw := make([]byte, 0, len(st.vec)*st.dims*4)
	buf := make([]byte, 4)
	off := 0
	for id, v := range st.vec {
		if len(v) != st.dims {
			continue
		}
		for _, f := range v {
			binary.LittleEndian.PutUint32(buf, math.Float32bits(f))
			raw = append(raw, buf...)
		}
		m.Notes[id] = slot{Hash: st.hash[id], Off: off}
		off++
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(binPath(dir), raw, 0o644); err != nil {
		return err
	}
	mb, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(metaPath(dir), mb, 0o644); err != nil {
		return err
	}
	memMu.Lock()
	delete(memBy, dir) // que la próxima lectura lo tome del disco recién escrito
	memMu.Unlock()
	return nil
}

// Stats describe el caché para poder inspeccionarlo sin abrir el binario.
type Stats struct {
	Notes int    `json:"notes"`
	Dims  int    `json:"dims"`
	Bytes int64  `json:"bytes"`
	Path  string `json:"path"`
}

// Describe informa qué hay guardado (cuántos vectores, de qué tamaño y cuánto
// ocupan), que es lo que uno realmente quiere saber de un caché opaco.
func Describe(dir string) Stats {
	st := load(dir)
	s := Stats{Notes: len(st.vec), Dims: st.dims, Path: binPath(dir)}
	if fi, err := os.Stat(binPath(dir)); err == nil {
		s.Bytes = fi.Size()
	}
	return s
}
