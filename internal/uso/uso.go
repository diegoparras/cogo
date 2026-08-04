// Package uso registra cuándo se consultó cada nota por última vez.
//
// # POR QUÉ FALTABA
//
// COGO sabía muchísimo de cada nota —qué la respalda, cuándo se verificó, de qué
// depende, quién la escribió— y nada sobre si a alguien le sirvió alguna vez.
// Eso deja sin responder la única pregunta que permite olvidar sin riesgo: si
// esto desapareciera, ¿lo extrañaría alguien?
//
// Sin ese dato, cualquier criterio de olvido termina siendo la edad. Y la edad
// es un pésimo criterio: la nota más vieja del vault puede ser la que más se
// consulta.
//
// # QUÉ CUENTA COMO CONSULTA
//
// Que la nota haya entrado en un `pack`, o que la hayan abierto por su id. Son
// los dos momentos en que un agente REALMENTE consume una nota.
//
// Aparecer en una búsqueda no cuenta. Una búsqueda devuelve candidatos, y contar
// candidatos convertiría a este registro en un contador de coincidencias léxicas:
// una nota con una palabra popular parecería utilísima sin que nadie la haya
// leído nunca.
//
// # DESDE CUÁNDO
//
// El archivo guarda su propia fecha de inicio, y es lo que hace que instalar
// esto no borre nada de golpe. Una nota sin registro no es una nota que nadie
// consultó: es una nota que nadie consultó DESDE QUE SE EMPEZÓ A MIRAR. La
// diferencia es todo — sin ella, el día que esta versión se despliega, cada nota
// vencida del vault se volvería latente a la vez.
package uso

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Registro es cuántas veces y cuándo se consultó una nota.
type Registro struct {
	Ultima time.Time `json:"ultima"`
	Veces  int       `json:"veces"`
}

type archivo struct {
	// Desde es cuándo empezó a registrarse. Ver el comentario del paquete: es lo
	// que impide que instalar esto declare latente a medio vault el primer día.
	Desde   time.Time            `json:"desde"`
	Notas   map[string]*Registro `json:"notas"`
	Version int                  `json:"version"`
}

// Store es el registro de uso de un vault.
type Store struct {
	ruta  string
	mu    sync.Mutex
	datos archivo
	sucio bool
	// ultimoGuardado agrupa las escrituras: un pack toca decenas de notas y no
	// tiene sentido escribir el archivo decenas de veces por petición.
	ultimoGuardado time.Time
	ahora          func() time.Time
}

// pausaGuardado es cuánto se espera antes de bajar los cambios a disco. Perder
// unos segundos de registro de uso no le hace daño a nadie; escribir en cada
// consulta sí se lo haría al disco.
const pausaGuardado = 5 * time.Second

// Abrir lee el registro de un vault. Uno que no existe arranca con la fecha de
// hoy, que es exactamente lo que corresponde: se empieza a mirar ahora.
func Abrir(vault string) *Store {
	s := &Store{
		ruta:  filepath.Join(vault, ".cogo", "uso.json"),
		ahora: time.Now,
		datos: archivo{Notas: map[string]*Registro{}, Version: 1},
	}
	b, err := os.ReadFile(s.ruta)
	if err != nil || json.Unmarshal(b, &s.datos) != nil {
		s.datos = archivo{Desde: time.Now().UTC(), Notas: map[string]*Registro{}, Version: 1}
		s.sucio = true
		return s
	}
	if s.datos.Notas == nil {
		s.datos.Notas = map[string]*Registro{}
	}
	if s.datos.Desde.IsZero() {
		s.datos.Desde = time.Now().UTC()
		s.sucio = true
	}
	return s
}

// SetReloj inyecta el tiempo. Solo para tests.
func (s *Store) SetReloj(f func() time.Time) { s.mu.Lock(); s.ahora = f; s.mu.Unlock() }

// Consultada anota que estas notas se usaron. Recibe varias porque un pack las
// entrega juntas, y así el archivo se toca una sola vez.
func (s *Store) Consultada(ids ...string) {
	if len(ids) == 0 {
		return
	}
	s.mu.Lock()
	ahora := s.ahora().UTC()
	for _, id := range ids {
		if id == "" {
			continue
		}
		r := s.datos.Notas[id]
		if r == nil {
			r = &Registro{}
			s.datos.Notas[id] = r
		}
		r.Ultima = ahora
		r.Veces++
		s.sucio = true
	}
	guardar := s.sucio && ahora.Sub(s.ultimoGuardado) >= pausaGuardado
	s.mu.Unlock()
	if guardar {
		_ = s.Guardar()
	}
}

// Ultima devuelve cuándo se consultó una nota, y si hay registro.
func (s *Store) Ultima(id string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.datos.Notas[id]
	if r == nil {
		return time.Time{}, false
	}
	return r.Ultima, true
}

// Veces devuelve cuántas veces se consultó.
func (s *Store) Veces(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.datos.Notas[id]; r != nil {
		return r.Veces
	}
	return 0
}

// Desde es cuándo se empezó a registrar.
func (s *Store) Desde() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.datos.Desde
}

// SinConsultar devuelve hace cuánto que no se consulta una nota. Cuando no hay
// registro, cuenta desde que se empezó a mirar — no desde que la nota existe.
func (s *Store) SinConsultar(id string, ahora time.Time) time.Duration {
	if u, hay := s.Ultima(id); hay {
		return ahora.Sub(u)
	}
	return ahora.Sub(s.Desde())
}

// Guardar baja el registro a disco.
func (s *Store) Guardar() error {
	s.mu.Lock()
	if !s.sucio {
		s.mu.Unlock()
		return nil
	}
	b, err := json.MarshalIndent(s.datos, "", "  ")
	s.sucio = false
	s.ultimoGuardado = s.ahora().UTC()
	ruta := s.ruta
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ruta, append(b, '\n'), 0o644)
}

// Olvidar saca del registro las notas que ya no existen, para que el archivo no
// crezca con ids de notas borradas.
func (s *Store) Olvidar(existen map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.datos.Notas {
		if !existen[id] {
			delete(s.datos.Notas, id)
			s.sucio = true
		}
	}
}
