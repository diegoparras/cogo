package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// El cerrojo entre procesos.
//
// # QUÉ ROMPE SIN ESTO
//
// El número de secuencia y el encadenado los lleva cada Journal EN MEMORIA. Con
// un solo proceso alcanza; con dos sobre el mismo vault, los dos creen que el
// último evento es el N y los dos escriben el N+1. El resultado no es un evento
// perdido —los dos quedan en el archivo— sino algo peor: dos eventos con el
// mismo número y dos ramas de la cadena de hashes. El registro deja de ser un
// registro.
//
// No es hipotético. Un despliegue rodante levanta el contenedor nuevo antes de
// bajar el viejo, y por unos segundos hay dos COGO con el mismo volumen montado.
//
// # POR QUÉ UN CERROJO DEL SISTEMA Y NO UN ARCHIVO CENTINELA
//
// La alternativa portable —crear un archivo con O_EXCL y borrarlo al terminar—
// tiene un defecto que la descalifica: si el proceso muere entre las dos cosas,
// el archivo queda, y TODAS las escrituras futuras se bloquean para siempre.
// Cambiar una corrupción rara por un vault trabado es un mal negocio, y
// destrabarlo requiere heurísticas de "cerrojo viejo" que aciertan a veces.
//
// Los cerrojos de flock y LockFileEx los libera el kernel cuando el proceso
// termina, muera como muera. No hay estado que limpiar.
//
// # LO QUE ESTE CERROJO NO CUBRE
//
// Es del sistema operativo, así que vale entre procesos de la MISMA máquina —
// que es el caso de dos contenedores sobre un volumen. Dos máquinas contra un
// NFS compartido no quedan protegidas: ahí el cerrojo de red no es confiable y
// haría falta otra cosa. COGO no está pensado para eso y conviene decirlo.

// esperaCerrojo es cuánto se espera antes de dar por trabado el registro.
//
// Que tenga límite es a propósito: esto corre dentro de un handler HTTP, y
// esperar para siempre por un proceso colgado dejaría la petición sin responder.
// Fallar con un mensaje claro es peor para quien escribe y mejor para todos los
// demás.
const esperaCerrojo = 5 * time.Second

type cerrojo struct{ f *os.File }

// bloquear toma el cerrojo exclusivo del registro, esperando si hace falta.
func bloquear(dir string, espera time.Duration) (*cerrojo, error) {
	ruta := filepath.Join(dir, ".cerrojo")
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("journal: no se pudo abrir el cerrojo %s: %w", ruta, err)
	}
	limite := time.Now().Add(espera)
	pausa := 200 * time.Microsecond
	for {
		ok, err := intentarCerrojo(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("journal: no se pudo tomar el cerrojo: %w", err)
		}
		if ok {
			return &cerrojo{f: f}, nil
		}
		if time.Now().After(limite) {
			f.Close()
			return nil, fmt.Errorf("journal: el registro está tomado por otro proceso hace más de %s. "+
				"¿Hay dos COGO sobre el mismo vault?", espera)
		}
		time.Sleep(pausa)
		if pausa < 20*time.Millisecond {
			pausa *= 2 // arranca fino para el caso normal, cede si de verdad hay que esperar
		}
	}
}

func (c *cerrojo) liberar() {
	if c == nil || c.f == nil {
		return
	}
	soltarCerrojo(c.f)
	c.f.Close()
}
