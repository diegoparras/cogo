// Package supervivencia deriva las ventanas de frescura de los datos del vault
// en vez de una tabla escrita a mano.
//
// # DE DÓNDE SALEN LOS 180 DÍAS
//
// De ningún lado. La tabla de frescura de COGO —constraint 365, decision 180,
// command 30— es una intuición razonable y nada más. Puede estar bien, pero
// nadie lo sabe, y en un vault donde los comandos cambian cada semana está mal
// de una forma que nadie va a notar: las notas se ven verdes hasta el día 30.
//
// El vault tiene la respuesta. Cada nota que fue desmentida, contradicha o
// corregida dice cuánto duró siendo cierta. Con suficientes, se puede estimar
// cuánto dura una nota de cada tipo y poner la ventana ahí.
//
// # POR QUÉ NO ES UN PROMEDIO
//
// Porque la mayoría de las notas no fallaron TODAVÍA. Promediar la duración de
// las que fallaron da un número absurdamente pesimista: es preguntar cuánto vive
// la gente encuestando velorios.
//
// Las notas que siguen vivas son observaciones CENSURADAS: no se sabe cuánto van
// a durar, pero sí que duraron al menos lo que llevan. Kaplan-Meier es
// exactamente el estimador que usa esa información parcial en vez de tirarla, y
// es la razón por la que este archivo tiene un estimador de supervivencia y no
// un promedio de tres líneas.
//
// # DÓNDE SE PONE LA VENTANA
//
// En el cuantil que el vault elija. Con el default de 20%: la ventana va donde
// la curva baja a 0,80 — o sea, fresca mientras 4 de cada 5 notas parecidas
// seguían siendo ciertas. Es una elección de tolerancia al riesgo, no un hecho,
// y por eso es un parámetro y no una constante.
//
// # CUÁNDO NO CONTESTA
//
// Cuando no hay suficientes notas de ese tipo, o cuando la curva nunca baja
// hasta el cuantil pedido (nadie falló todavía). Ahí devuelve "sin datos" y el
// tipo se sigue rigiendo por la tabla. Una estimación sobre cuatro casos sería
// peor que la intuición que reemplaza.
package supervivencia

import (
	"sort"
)

// Observacion es una nota que vivió: cuántos días, y si murió o sigue viva.
type Observacion struct {
	Tipo string
	Dias int
	// Fallo es true si la nota fue desmentida, contradicha o corregida. False es
	// una observación censurada: sigue viva, y lo único que se sabe es que duró
	// al menos Dias.
	Fallo bool
}

// Estimacion es la respuesta para un tipo de nota.
type Estimacion struct {
	Tipo string `json:"tipo"`
	// Ventana es el resultado: los días que la nota se considera fresca. 0 si no
	// se pudo estimar.
	Ventana int `json:"ventana"`
	// Los tres números que hacen falta para creerle o no.
	Observaciones int `json:"observaciones"`
	Fallos        int `json:"fallos"`
	Vivas         int `json:"vivas"`
	// Suficiente dice si hay datos como para usarla.
	Suficiente bool `json:"suficiente"`
	// Motivo explica por qué no hay estimación, cuando no la hay.
	Motivo string `json:"motivo,omitempty"`
	// Curva son los puntos de Kaplan-Meier, para poder dibujarla y discutirla.
	Curva []Punto `json:"curva,omitempty"`
}

// Punto es un escalón de la curva de supervivencia.
type Punto struct {
	Dias  int     `json:"dias"`
	Vivas float64 `json:"vivas"` // proporción que sigue siendo cierta
}

// Estimar calcula la ventana de cada tipo presente en las observaciones.
//
// minimo es cuántas observaciones hacen falta para contestar; cuantil es el
// porcentaje de fallos que se acepta antes de pedir revisión.
func Estimar(obs []Observacion, minimo, cuantil int) map[string]Estimacion {
	porTipo := map[string][]Observacion{}
	for _, o := range obs {
		if o.Dias < 0 {
			continue
		}
		porTipo[o.Tipo] = append(porTipo[o.Tipo], o)
	}
	out := map[string]Estimacion{}
	for tipo, os := range porTipo {
		out[tipo] = estimarUno(tipo, os, minimo, cuantil)
	}
	return out
}

func estimarUno(tipo string, obs []Observacion, minimo, cuantil int) Estimacion {
	e := Estimacion{Tipo: tipo, Observaciones: len(obs)}
	for _, o := range obs {
		if o.Fallo {
			e.Fallos++
		} else {
			e.Vivas++
		}
	}
	if len(obs) < minimo {
		e.Motivo = "todavía no hay suficientes notas de este tipo como para estimar nada"
		return e
	}
	if e.Fallos == 0 {
		e.Motivo = "ninguna nota de este tipo falló todavía: no hay curva que estimar"
		return e
	}

	e.Curva = kaplanMeier(obs)
	objetivo := 1 - float64(cuantil)/100
	for _, p := range e.Curva {
		if p.Vivas <= objetivo {
			e.Ventana = p.Dias
			e.Suficiente = true
			return e
		}
	}
	e.Motivo = "la curva no baja hasta el punto de corte: las notas de este tipo duran más de lo observado"
	return e
}

// kaplanMeier estima la proporción que sigue siendo cierta a lo largo del
// tiempo, usando las observaciones censuradas en vez de descartarlas.
//
// En cada tiempo con fallos: S = S × (1 − fallos / en_riesgo). Lo que hace que
// funcione es "en riesgo": una nota censurada cuenta en el denominador hasta el
// momento en que se la dejó de observar, y desde ahí sale sin contar como fallo.
func kaplanMeier(obs []Observacion) []Punto {
	os := append([]Observacion(nil), obs...)
	sort.SliceStable(os, func(i, j int) bool {
		if os[i].Dias != os[j].Dias {
			return os[i].Dias < os[j].Dias
		}
		// A igual tiempo, los fallos se procesan antes que las censuras: la
		// convención estándar, y la conservadora.
		return os[i].Fallo && !os[j].Fallo
	})

	var curva []Punto
	s := 1.0
	enRiesgo := len(os)
	for i := 0; i < len(os); {
		t := os[i].Dias
		fallos, salen := 0, 0
		for i < len(os) && os[i].Dias == t {
			if os[i].Fallo {
				fallos++
			}
			salen++
			i++
		}
		if fallos > 0 && enRiesgo > 0 {
			s *= 1 - float64(fallos)/float64(enRiesgo)
			curva = append(curva, Punto{Dias: t, Vivas: s})
		}
		enRiesgo -= salen
	}
	return curva
}
