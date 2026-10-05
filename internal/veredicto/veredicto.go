// Package veredicto guarda lo que el humano dijo de cada decisión de COGO.
//
// # POR QUÉ
//
// COGO ya cuenta tokens ahorrados. Falta el número que le mostrás a alguien
// para que lo adopte: cuántas veces `authorize` dijo que no y el humano
// confirmó que tenía razón. Sin eso, un bloqueo es una molestia; con eso, es
// un acierto o un error, y la tasa entre los dos es la única medida honesta
// de si el control sirve.
//
// Un veredicto se ata a un recibo (internal/recibo): la decisión concreta, con
// su contexto, no una fila del log.
package veredicto

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/atomico"
)

// Veredicto es lo que una persona dijo de un recibo.
type Veredicto struct {
	Recibo   string    `json:"recibo"`
	Acertado bool      `json:"acertado"` // COGO tenía razón
	Quien    string    `json:"quien"`
	Cuando   time.Time `json:"cuando"`
	Nota     string    `json:"nota,omitempty"`
}

type Store struct {
	ruta string
	mu   sync.Mutex
}

func Abrir(vault string) *Store {
	return &Store{ruta: filepath.Join(vault, ".cogo", "veredictos.json")}
}

func (s *Store) leer() (map[string]Veredicto, error) {
	out := map[string]Veredicto{}
	b, err := os.ReadFile(s.ruta)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Poner asienta (o cambia) el veredicto de un recibo. Cambiar de opinión es
// legítimo: gana el último.
func (s *Store) Poner(v Veredicto) error {
	if strings.TrimSpace(v.Recibo) == "" {
		return fmt.Errorf("veredicto: sin recibo no hay sobre qué opinar")
	}
	if v.Cuando.IsZero() {
		v.Cuando = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	todos, err := s.leer()
	if err != nil {
		return err
	}
	todos[v.Recibo] = v
	b, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return err
	}
	return atomico.Escribir(s.ruta, b, 0o644)
}

// Todos devuelve los veredictos por recibo.
func (s *Store) Todos() (map[string]Veredicto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leer()
}

// Balance es lo que se muestra: cuántos bloqueos juzgó el humano y cuántos
// fueron aciertos, y lo mismo para lo que pasó.
type Balance struct {
	Juzgados      int `json:"juzgados"`
	BloqueosBien  int `json:"bloqueos_bien"` // COGO frenó y el humano confirmó
	BloqueosMal   int `json:"bloqueos_mal"`  // COGO frenó y no debía
	PermisosBien  int `json:"permisos_bien"`
	PermisosMal   int `json:"permisos_mal"` // COGO dejó pasar y no debía: el peor caso
	SinJuzgar     int `json:"sin_juzgar"`
	TotalDecidido int `json:"total_decidido"`
}

// Balancear cruza recibos (id → autorizó) con veredictos.
func Balancear(autorizo map[string]bool, vs map[string]Veredicto) Balance {
	var b Balance
	b.TotalDecidido = len(autorizo)
	for id, ok := range autorizo {
		v, hay := vs[id]
		if !hay {
			b.SinJuzgar++
			continue
		}
		b.Juzgados++
		switch {
		case !ok && v.Acertado:
			b.BloqueosBien++
		case !ok && !v.Acertado:
			b.BloqueosMal++
		case ok && v.Acertado:
			b.PermisosBien++
		default:
			b.PermisosMal++
		}
	}
	return b
}
