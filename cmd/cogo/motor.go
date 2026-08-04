package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/motor"
)

// instalarMotor conecta el motor de confianza basado en el journal.
//
// El cambio ocurre en un solo lugar: core.SetMotor. Los diez llamadores
// repartidos entre el visor, la CLI y el servidor MCP siguen llamando a
// core.EvaluateVault y no se enteran de nada — que es lo que hace que esto se
// pueda revertir en caliente.
//
// COGO_MOTOR=legacy vuelve al anterior. Es la marcha atrás: si algo sale mal en
// una instancia que ya está andando, se arregla con una variable de entorno y un
// reinicio, sin esperar a que salga una versión.
func instalarMotor(dir string) error {
	if motor.Legacy() {
		log.Printf("cogo: motor de confianza ANTERIOR (COGO_MOTOR=legacy)")
		return nil
	}

	j, err := journal.Open(dir)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el registro de eventos: %w", err)
	}
	// La cadena está rota si alguien editó un evento pasado. Se avisa y se
	// sigue: negarse a arrancar dejaría a alguien sin su vault por una línea
	// corrupta, y el aviso es lo que hace falta para investigarlo.
	if err := j.Verificar(); err != nil {
		log.Printf("cogo: ATENCIÓN — %v", err)
	}

	var (
		mu       sync.Mutex
		sembrado bool
	)
	core.SetMotor(func(vault map[string]*core.Note, contras map[string]bool, hoy core.Date) map[string]core.Verdict {
		mu.Lock()
		defer mu.Unlock()

		// La primera evaluación ancla las notas que ya existían: sin eso el
		// registro arrancaría vacío y todo el vault aparecería sin respaldo.
		// Es idempotente, así que reiniciar no duplica nada.
		if !sembrado {
			previos := core.EvaluateVaultCore(vault, contras, hoy)
			if n, err := journal.Sembrar(j, vault, previos); err != nil {
				log.Printf("cogo: no se pudo sembrar el registro: %v", err)
			} else if n > 0 {
				log.Printf("cogo: registro sembrado con %d notas", n)
			}
			sembrado = true
		}
		// Notas nuevas creadas después del arranque: se anclan al vuelo.
		if faltan(j, vault) {
			previos := core.EvaluateVaultCore(vault, contras, hoy)
			_, _ = journal.Sembrar(j, vault, previos)
		}

		evs, err := j.All()
		if err != nil {
			log.Printf("cogo: no se pudo leer el registro (%v); se usa el motor anterior", err)
			return core.EvaluateVaultCore(vault, contras, hoy)
		}
		// El eje de frescura necesita el StaleAt, que lo calcula la tabla de
		// ventanas por tipo del motor anterior. Es un dato de la nota, no del
		// registro.
		previos := core.EvaluateVaultCore(vault, contras, hoy)
		for id, n := range vault {
			n.StaleAt = previos[id].StaleAt
		}
		return motor.Evaluar(vault, contras, hoy, evs)
	})
	log.Printf("cogo: motor de confianza sobre el registro de eventos")
	return nil
}

// faltan dice si hay notas del vault que el registro todavía no conoce.
func faltan(j *journal.Journal, vault map[string]*core.Note) bool {
	evs, err := j.All()
	if err != nil {
		return false
	}
	conocidas := make(map[string]bool, len(evs))
	for _, e := range evs {
		conocidas[e.NoteID] = true
	}
	for id := range vault {
		if !conocidas[id] {
			return true
		}
	}
	return false
}
