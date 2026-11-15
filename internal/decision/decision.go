// Package decision guarda qué hizo el humano cuando COGO no autorizó algo.
//
// # POR QUÉ
//
// Un NOT AUTHORIZED que solo frena deja al agente sin salida y al humano sin
// enterarse: el agente lo rodea o se queda trabado. Lo que el plan pedía es
// que la decisión vuelva a quien corresponde. Donde el harness sabe preguntar,
// COGO pregunta, y la respuesta queda asentada al lado del recibo.
//
// # LA MÁQUINA
//
// Es la cola de aprobación de autorizacion.yaml, en el plan de septiembre:
//
//	pendiente --aprobada------------------> aprobada
//	pendiente --rechazada-----------------> rechazada
//	pendiente --sin respuesta en Vence----> vencida
//
// Las tres de la derecha son terminales: una vencida no vuelve a aprobada
// aunque la aprobación llegue tarde, y una rechazada no cambia de opinión por
// un evento posterior. El estado no se guarda: se pliega de los eventos cada
// vez que se lee, así que la regla vale también para el pasado.
//
// Va en su propio archivo append-only, `.cogo/decisiones.jsonl`, y no en el
// journal: el journal pliega cada evento como el de una nota, y una decisión
// no es una nota. El recibo es el que ata la decisión a lo que se sabía.
package decision

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

type Estado string

const (
	Pendiente Estado = "pendiente"
	Aprobada  Estado = "aprobada"
	Rechazada Estado = "rechazada"
	Vencida   Estado = "vencida"
)

// Por dónde se le preguntó al humano.
const (
	ViaHook     = "hook"     // el diálogo de permiso del harness
	ViaPregunta = "pregunta" // la tool le pidió confirmación por MCP
)

// Vence es cuánto espera un pedido. Un diálogo de permiso se contesta en
// minutos; si en media hora no llegó la aprobación, el humano lo rechazó o no
// lo vio, y para COGO las dos cosas son lo mismo: no se aprobó.
const Vence = 30 * time.Minute

// Pedido abre una decisión: COGO no autorizó y se le preguntó al humano.
type Pedido struct {
	Recibo    string
	ToolUseID string // para cruzarlo con lo que el harness ejecutó después
	Accion    string
	Via       string
	Cuando    time.Time
}

// Decision es el estado plegado de un recibo.
type Decision struct {
	Estado   Estado    `json:"estado"`
	Accion   string    `json:"accion"`
	Via      string    `json:"via"`
	Quien    string    `json:"quien,omitempty"`
	Pedida   time.Time `json:"pedida"`
	Resuelta time.Time `json:"resuelta,omitempty"`
}

// evento es una línea del archivo.
type evento struct {
	Recibo    string    `json:"recibo"`
	Estado    Estado    `json:"estado"`
	ToolUseID string    `json:"tool_use_id,omitempty"`
	Accion    string    `json:"accion,omitempty"`
	Via       string    `json:"via,omitempty"`
	Quien     string    `json:"quien,omitempty"`
	Cuando    time.Time `json:"cuando"`
}

type Store struct {
	ruta string
	mu   sync.Mutex
}

func Abrir(vault string) *Store {
	return &Store{ruta: filepath.Join(vault, ".cogo", "decisiones.jsonl")}
}

// Pedir asienta que se le preguntó al humano.
func (s *Store) Pedir(p Pedido) error {
	if strings.TrimSpace(p.Recibo) == "" {
		return fmt.Errorf("decisión: sin recibo no hay qué decidir")
	}
	return s.agregar(evento{Recibo: p.Recibo, Estado: Pendiente, ToolUseID: p.ToolUseID, Accion: p.Accion, Via: p.Via, Cuando: p.Cuando})
}

// Resolver asienta la respuesta. Solo se puede resolver lo que se pidió; si ya
// estaba resuelto o vencido, el evento queda escrito pero el pliegue lo ignora.
func (s *Store) Resolver(recibo string, e Estado, quien string, cuando time.Time) error {
	if e != Aprobada && e != Rechazada {
		return fmt.Errorf("decisión: %q no es una respuesta", e)
	}
	if _, ok := s.Estados(cuando)[recibo]; !ok {
		return fmt.Errorf("decisión: nunca se pidió %s", recibo)
	}
	return s.agregar(evento{Recibo: recibo, Estado: e, Quien: quien, Cuando: cuando})
}

// PorToolUse devuelve el recibo pendiente de una llamada del harness.
func (s *Store) PorToolUse(id string) (string, bool) {
	if strings.TrimSpace(id) == "" {
		return "", false
	}
	evs, _ := s.leer()
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Estado == Pendiente && evs[i].ToolUseID == id {
			return evs[i].Recibo, true
		}
	}
	return "", false
}

// Estados pliega los eventos al instante `ahora`.
func (s *Store) Estados(ahora time.Time) map[string]Decision {
	evs, _ := s.leer()
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Cuando.Before(evs[j].Cuando) })
	out := map[string]Decision{}
	for _, ev := range evs {
		d, hay := out[ev.Recibo]
		if ev.Estado == Pendiente {
			if !hay {
				out[ev.Recibo] = Decision{Estado: Pendiente, Accion: ev.Accion, Via: ev.Via, Pedida: ev.Cuando}
			}
			continue
		}
		if !hay || d.Estado != Pendiente || ev.Cuando.Sub(d.Pedida) > Vence {
			continue // terminal, o la respuesta llegó tarde
		}
		d.Estado, d.Quien, d.Resuelta = ev.Estado, ev.Quien, ev.Cuando
		out[ev.Recibo] = d
	}
	for id, d := range out {
		if d.Estado == Pendiente && ahora.Sub(d.Pedida) > Vence {
			d.Estado = Vencida
			out[id] = d
		}
	}
	return out
}

func (s *Store) agregar(ev evento) error {
	if ev.Cuando.IsZero() {
		ev.Cuando = time.Now().UTC()
	}
	b, err := json.Marshal(ev)
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

func (s *Store) leer() ([]evento, error) {
	s.mu.Lock()
	b, err := os.ReadFile(s.ruta)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	var out []evento
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var ev evento
		if json.Unmarshal([]byte(ln), &ev) == nil {
			out = append(out, ev)
		}
	}
	return out, nil
}
