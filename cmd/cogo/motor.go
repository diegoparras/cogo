package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/calibracion"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/diegoparras/cogo/internal/parametros"
	"github.com/diegoparras/cogo/internal/supervivencia"
	"github.com/diegoparras/cogo/internal/uso"
)

// pars son los parámetros vigentes de este proceso. Los carga instalarMotor y
// los comparte con el visor, que es quien los edita: un solo Set, así lo que se
// guarda desde el panel es exactamente lo que el motor lee en la evaluación
// siguiente, sin reiniciar.
var pars = parametros.Defaults()

// instalarParametros carga el registro y engancha lo que depende de él. Se llama
// antes que nada: la tabla de frescura la usan hasta los comandos que no abren
// el motor de eventos.
func instalarParametros(dir string) {
	pars = parametros.Cargar(dir)
	core.SetVentanas(func(tipo string) (int, bool) {
		// Primero, si el vault decidió derivar las ventanas de sus datos y para
		// ESE tipo hay con qué. La estimación se calcula aparte y se cachea; acá
		// solo se consulta.
		if v, ok := ventanaEstimada(tipo); ok {
			return v, true
		}
		clave := "frescura." + tipo
		switch tipo {
		case "constraint", "decision", "architecture", "runbook", "bug", "command":
		default:
			clave = "frescura.otros"
		}
		return pars.Entero(clave), true
	})
	core.SetCaracteresDistintivos(func() int { return pars.Entero("ancla.caracteres_minimos") })

	// El olvido necesita saber qué se consulta. Sin este registro, core.Latente
	// no tiene con qué decidir y deja todo en circulación, que es el
	// comportamiento correcto ante la falta de datos.
	registroUso = uso.Abrir(dir)
	core.SetUso(func(id string, ahora time.Time) time.Duration {
		return registroUso.SinConsultar(id, ahora)
	})
	core.SetDiasSinConsultar(func() int { return pars.Entero("olvido.dias_sin_consultar") })
}

// registroUso es el registro de consultas del vault, compartido por el proceso.
var registroUso = uso.Abrir("")

// Consultadas anota que estas notas se entregaron a alguien. Es lo que despierta
// a una nota latente: consultarla la saca de "sin consultar", y como la latencia
// se calcula y no se escribe, con eso vuelve al camino sola.
func Consultadas(ids ...string) { registroUso.Consultada(ids...) }

// registro es el journal del vault, compartido por todo el proceso.
//
// Está acá y no dentro de instalarMotor porque abrir un journal NO es barato:
// Open lee y parsea el registro entero para ponerse al día con la cadena. Con
// veinte mil eventos eso son setenta milisegundos, y `authorize` lo pagaba en
// CADA llamada — más otra lectura completa después.
//
// Un solo handle, además, es un solo caché de lectura: lo que capturó una
// herramienta ya está en memoria cuando lo consulta la siguiente.
var (
	muRegistro sync.Mutex
	registro   *journal.Journal
)

// journalDe devuelve el registro compartido, abriéndolo la primera vez. Sirve
// para los caminos que no pasan por instalarMotor —COGO_MOTOR=legacy, un
// subcomando suelto— sin que ninguno tenga que saber cuál fue.
func journalDe(dir string) (*journal.Journal, error) {
	muRegistro.Lock()
	defer muRegistro.Unlock()
	if registro != nil {
		return registro, nil
	}
	j, err := journal.Open(dir)
	if err != nil {
		return nil, err
	}
	registro = j
	return j, nil
}

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

	j, err := journalDe(dir)
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
		evs, err := j.All()
		if err != nil {
			log.Printf("cogo: no se pudo leer el registro (%v); se usa el motor anterior", err)
			return core.EvaluateVaultCore(vault, contras, hoy)
		}
		// Notas nuevas creadas después del arranque: se anclan al vuelo. Se
		// decide con los eventos que ya se leyeron, en vez de volver a leer.
		if faltan(evs, vault) {
			previos := core.EvaluateVaultCore(vault, contras, hoy)
			if _, err := journal.Sembrar(j, vault, previos); err == nil {
				evs, _ = j.All() // sembrar agregó eventos: hay que verlos
			}
		}
		// El eje de frescura necesita el StaleAt, que lo calcula la tabla de
		// ventanas por tipo del motor anterior. Es un dato de la nota, no del
		// registro.
		previos := core.EvaluateVaultCore(vault, contras, hoy)
		for id, n := range vault {
			n.StaleAt = previos[id].StaleAt
		}
		refrescarEstimaciones(vault, evs)
		// El registro de consultas se limpia de las notas que ya no están. Va
		// acá porque es el único punto que ve el vault entero en cada pasada, y
		// sin esto uso.json acumularía para siempre el id de cada nota borrada.
		existen := make(map[string]bool, len(vault))
		for id := range vault {
			existen[id] = true
		}
		registroUso.Olvidar(existen)
		return motor.EvaluarCon(vault, contras, hoy, evs, motor.Opciones{
			Penalizados: emisoresPenalizados(evs),
		})
	})
	log.Printf("cogo: motor de confianza sobre el registro de eventos")
	return nil
}

// emisoresPenalizados devuelve a quiénes dejó de alcanzarles la palabra. Con la
// calibración apagada —el default— devuelve vacío sin calcular nada: el módulo
// existe, mira y reporta, pero no toca ningún color hasta que alguien lo
// encienda a sabiendas.
func emisoresPenalizados(evs []journal.Event) map[string]bool {
	if !pars.Bool("calibracion.activa") {
		return nil
	}
	return calibracion.Penalizados(calibracion.Calcular(evs,
		pars.Entero("calibracion.minimo_declaraciones"),
		pars.Entero("calibracion.desmentidas_toleradas")))
}

// Las ventanas estimadas se recalculan en cada evaluación y se guardan acá. No
// es un caché por performance —es barato— sino por ORDEN: estimar necesita el
// vault entero, y core.windowDays se pregunta por una nota a la vez.
var (
	muEst        sync.RWMutex
	estimaciones map[string]supervivencia.Estimacion
)

func refrescarEstimaciones(vault map[string]*core.Note, evs []journal.Event) {
	if !pars.Bool("supervivencia.activa") {
		return
	}
	est := supervivencia.Estimar(
		supervivencia.Observar(vault, evs, time.Now()),
		pars.Entero("supervivencia.minimo_observaciones"),
		pars.Entero("supervivencia.cuantil"))
	muEst.Lock()
	estimaciones = est
	muEst.Unlock()
}

func ventanaEstimada(tipo string) (int, bool) {
	if !pars.Bool("supervivencia.activa") {
		return 0, false
	}
	muEst.RLock()
	defer muEst.RUnlock()
	e, ok := estimaciones[tipo]
	if !ok || !e.Suficiente || e.Ventana <= 0 {
		return 0, false
	}
	return e.Ventana, true
}

// faltan dice si hay notas del vault que el registro todavía no conoce. Recibe
// los eventos ya leídos: es la misma lectura que necesita el motor, y pedirla
// dos veces era duplicar el trabajo de la evaluación entera.
func faltan(evs []journal.Event, vault map[string]*core.Note) bool {
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
