package confidence

import "sort"

// La propagación de la duda por el grafo de dependencias, como punto fijo sobre
// el retículo.
//
// POR QUÉ EL MAYOR PUNTO FIJO Y NO EL MENOR
//
// La función de propagación
//
//	F(σ)[n] = meet( local(n), meet_{d ∈ deps(n)} σ[d] )
//
// es monótona sobre un retículo completo finito, así que por Knaster-Tarski
// tiene TANTO un menor como un mayor punto fijo. El teorema garantiza que
// existen los dos; cuál se usa es una decisión semántica, no una consecuencia.
//
// COGO usa el MAYOR, que se alcanza iterando hacia abajo desde el elemento
// máximo. La diferencia se ve justo donde importa: en un ciclo A↔B donde ninguna
// de las dos tiene motivo propio para degradarse, el mayor punto fijo las deja
// como están, y el menor las hundiría hasta el fondo del retículo solo por
// referenciarse entre sí. Esto último sería peor que el motor actual, que ya las
// castiga de más poniéndolas en rojo.
//
// Dicho en la jerga del análisis de flujo de datos: es la interpretación "must",
// no la "may". Una nota se sostiene mientras nada demuestre lo contrario.
//
// TERMINACIÓN
//
// Cada nota solo puede BAJAR de estado, y el retículo tiene altura finita, así
// que cada nota cambia a lo sumo altura(retículo) veces. El total de vueltas
// está acotado por |notas| × altura, sin importar cuántos ciclos haya ni por
// dónde se empiece. No hace falta detectar ciclos: convergen solos.

// Grafo describe de qué depende cada nota.
type Grafo map[string][]string

// PuntoFijo calcula el mayor punto fijo de la propagación.
//
// `local` es el estado de cada nota por sus propios méritos, ya combinados todos
// los ejes que no dependen de otras notas (el check, la evidencia, la frescura,
// las contradicciones). Una dependencia que no está en `local` se trata como lo
// menos confiable posible: apoyarse en algo que no se puede ver no es apoyarse.
func PuntoFijo(g Grafo, local map[string]Estado) map[string]Estado {
	// Se arranca desde el elemento máximo. Iterar hacia abajo desde ⊤ es lo que
	// converge al MAYOR punto fijo.
	sigma := make(map[string]Estado, len(local))
	for id := range local {
		sigma[id] = Verified
	}

	// Índice inverso: quién depende de quién. Cuando una nota baja, solo hay
	// que revisar a los que se apoyan en ella.
	dependientes := map[string][]string{}
	for id, deps := range g {
		for _, d := range deps {
			dependientes[d] = append(dependientes[d], id)
		}
	}

	// La worklist arranca con todas las notas, en orden estable. El orden no
	// cambia el resultado —eso es lo que prueban los tests— pero sí hace que
	// dos corridas recorran los mismos pasos, que ayuda al depurar.
	pendientes := make([]string, 0, len(local))
	for id := range local {
		pendientes = append(pendientes, id)
	}
	sort.Strings(pendientes)
	enCola := make(map[string]bool, len(pendientes))
	for _, id := range pendientes {
		enCola[id] = true
	}

	for len(pendientes) > 0 {
		id := pendientes[0]
		pendientes = pendientes[1:]
		enCola[id] = false

		nuevo := local[id]
		for _, d := range g[id] {
			est, existe := sigma[d]
			if !existe {
				// Depende de una nota que no está en el vault.
				est = Estado(0) // el fondo del retículo
			}
			nuevo = Meet(nuevo, est)
		}
		if nuevo == sigma[id] {
			continue
		}
		sigma[id] = nuevo
		// Solo puede haber bajado (F es decreciente desde ⊤), así que hay que
		// revisar a quienes se apoyan en esta nota.
		for _, dep := range dependientes[id] {
			if !enCola[dep] {
				pendientes = append(pendientes, dep)
				enCola[dep] = true
			}
		}
	}
	return sigma
}

// Altura es la cantidad de niveles del retículo. Acota cuántas veces puede
// cambiar el estado de una nota, y con eso la terminación del punto fijo.
func Altura() int {
	n := 0
	for e := Estado(0); int(e) < 64; e++ {
		if e.String() == "desconocido" {
			break
		}
		if !e.Transitorio() {
			n++
		}
	}
	return n
}
