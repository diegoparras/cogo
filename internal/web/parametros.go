package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/calibracion"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/parametros"
	"github.com/diegoparras/cogo/internal/supervivencia"
)

// El modo deidad: la superficie de control completa del motor.
//
// La decisión de diseño está en QUÉ se muestra, no en cómo. Se muestra todo. No
// un subconjunto seguro, no "opciones avanzadas" a medias: los diecinueve
// números que gobiernan el sistema, con su valor, su rango, de dónde salen y qué
// se afloja si se mueven.
//
// Que esté todo es lo que hace legítimo que por default no se vea NADA. Esconder
// controles porque el usuario podría lastimarse es condescendiente; no
// necesitarlos porque los defaults son buenos es diseño. La diferencia se nota
// justo acá: el día que alguien SÍ necesita el control, está entero.

// UsarParametros conecta el visor al mismo Set que lee el motor. Es la misma
// instancia a propósito: lo que se guarda desde el panel tiene que valer en la
// evaluación siguiente, sin reiniciar nada.
func (s *Server) UsarParametros(p *parametros.Set) { s.pars = p }

func (s *Server) parametros() *parametros.Set {
	if s.pars == nil {
		return parametros.Defaults()
	}
	return s.pars
}

type grupoJSON struct {
	Clave  string              `json:"clave"`
	Titulo string              `json:"titulo"`
	Params []parametros.Estado `json:"params"`
}

func (s *Server) handleParametros(w http.ResponseWriter, r *http.Request) {
	p := s.parametros()
	if r.Method == http.MethodGet {
		lista := p.Listar()
		var grupos []grupoJSON
		for _, g := range parametros.GruposOrdenados {
			gr := grupoJSON{Clave: g, Titulo: parametros.TituloGrupo[g]}
			for _, e := range lista {
				if e.Grupo == g {
					gr.Params = append(gr.Params, e)
				}
			}
			if len(gr.Params) > 0 {
				grupos = append(grupos, gr)
			}
		}
		writeJSON(w, map[string]any{
			"grupos": grupos, "editados": p.Editados(), "total": len(lista),
		})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var in struct {
		Clave  string `json:"clave"`
		Valor  any    `json:"valor"`
		Volver bool   `json:"restaurar,omitempty"`
		Todo   bool   `json:"restaurar_todo,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	quien := auth.Caller(r)
	if quien == "" {
		quien = "visor"
	}

	var err error
	switch {
	case in.Todo:
		p.RestaurarTodo(quien)
	case in.Volver:
		err = p.Restaurar(in.Clave, quien)
	default:
		err = p.Poner(in.Clave, in.Valor, quien)
	}
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if err := p.Guardar(); err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	// El color de cada nota puede haber cambiado —mover una ventana de frescura
	// mueve el semáforo— así que el visor tiene que recargar lo que muestra.
	writeJSON(w, map[string]any{"ok": true, "editados": p.Editados()})
}

// handleSalud es lo que el sistema aprendió del vault pero todavía no está
// usando.
//
// Existe porque la calibración y las ventanas por supervivencia vienen apagadas,
// y un módulo apagado e invisible es un módulo que no existe. Acá se ve qué
// diría si estuviera encendido, con cuántos datos, y qué le falta para poder
// decir algo — que es la información con la que alguien decide si encenderlo.
func (s *Server) handleSalud(w http.ResponseWriter, r *http.Request) {
	p := s.parametros()
	out := map[string]any{
		"calibracion_activa":   p.Bool("calibracion.activa"),
		"supervivencia_activa": p.Bool("supervivencia.activa"),
	}

	vault, err := s.cache.Load()
	if err != nil {
		writeJSON(w, out)
		return
	}
	j, err := journal.Open(s.dir)
	if err != nil {
		writeJSON(w, out)
		return
	}
	evs, err := j.All()
	if err != nil {
		writeJSON(w, out)
		return
	}

	inf := calibracion.Calcular(evs,
		p.Entero("calibracion.minimo_declaraciones"),
		p.Entero("calibracion.desmentidas_toleradas"))
	inf.Activa = p.Bool("calibracion.activa")
	out["calibracion"] = inf

	est := supervivencia.Estimar(
		supervivencia.Observar(vault, evs, time.Now()),
		p.Entero("supervivencia.minimo_observaciones"),
		p.Entero("supervivencia.cuantil"))

	// La comparación es lo que se quiere ver: qué días usa hoy cada tipo, y qué
	// días dirían los datos. Sin las dos columnas al lado, la estimación es un
	// número suelto que no se puede juzgar.
	type ventana struct {
		Tipo      string `json:"tipo"`
		EnUso     int    `json:"en_uso"`
		Estimada  int    `json:"estimada,omitempty"`
		Observ    int    `json:"observaciones"`
		Fallos    int    `json:"fallos"`
		Motivo    string `json:"motivo,omitempty"`
		Aplicable bool   `json:"aplicable"`
	}
	var vs []ventana
	for _, tipo := range []string{"constraint", "decision", "architecture", "runbook", "bug", "command"} {
		v := ventana{Tipo: tipo, EnUso: p.Entero("frescura." + tipo)}
		if e, ok := est[tipo]; ok {
			v.Estimada, v.Observ, v.Fallos, v.Motivo, v.Aplicable = e.Ventana, e.Observaciones, e.Fallos, e.Motivo, e.Suficiente
		} else {
			v.Motivo = "no hay notas de este tipo en el vault"
		}
		vs = append(vs, v)
	}
	out["ventanas"] = vs
	out["notas"] = len(vault)
	out["eventos"] = len(evs)
	writeJSON(w, out)
}
