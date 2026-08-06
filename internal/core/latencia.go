package core

import (
	"fmt"
	"time"
)

// La latencia: cómo COGO olvida.
//
// # LA DEUDA QUE PAGA
//
// COGO calificaba todo lo que entraba y no sacaba nada nunca. El color aísla lo
// rojo pero no lo elimina: sigue existiendo, sigue costando, sigue apareciendo.
// Un vault a tres años son miles de notas, casi todas vencidas, todas ahí.
//
// Y no es un problema de espacio. Es que lo muerto tapa a lo vivo: buscar se
// vuelve ruidoso, el visor deja de servir para mirar, y la nota que importa está
// entre cuatrocientas que no.
//
// # POR QUÉ NO ALCANZA CON LA EDAD
//
// Porque la nota más vieja del vault puede ser la que más se consulta. Olvidar
// por antigüedad es el error obvio, y produce exactamente el daño que uno quiere
// evitar: perder lo que sí se usaba.
//
// COGO puede hacerlo mejor porque sabe cosas que un contador de fechas no: qué
// notas tienen dependientes, cuáles se consultaron, cuáles ya vencieron, cuáles
// están contradichas. Una nota se vuelve latente cuando TODAS dan lo mismo.
//
// # LATENTE NO ES BORRADA
//
// La nota sigue en el vault, sigue siendo un archivo, sigue abriéndose por su
// id. Lo que cambia es que deja de entrar en el `pack` y en las búsquedas: sale
// del camino, no de la historia.
//
// # Y SE CALCULA, NO SE ESCRIBE
//
// Igual que el color. Nadie marca una nota como latente: la condición se evalúa
// cada vez. Eso hace que despertar sea automático y sin ceremonia — consultá la
// nota y deja de estar sin consultar, y por lo tanto deja de ser latente. No hay
// un estado que alguien tenga que acordarse de revertir.

// uso, si está instalado, dice hace cuánto que no se consulta una nota. Mismo
// patrón de costura que SetMotor y SetVentanas: core no lee archivos.
var uso func(id string, ahora time.Time) time.Duration

// SetUso instala el registro de consultas. Sin él no hay latencia: sin saber qué
// se usa, olvidar sería adivinar.
func SetUso(f func(id string, ahora time.Time) time.Duration) { uso = f }

// diasSinConsultar, si está instalado, es el umbral configurado del vault.
var diasSinConsultar func() int

// SetDiasSinConsultar instala el umbral. 0 apaga la latencia.
func SetDiasSinConsultar(f func() int) { diasSinConsultar = f }

func umbralSinConsultar() int {
	if diasSinConsultar != nil {
		return diasSinConsultar()
	}
	return 0 // sin configurar, no se olvida nada
}

// Latencia explica por qué una nota está (o no está) latente. La razón importa
// tanto como el veredicto: alguien que ve una nota fuera del camino tiene que
// poder saber qué la sacó.
type Latencia struct {
	Latente bool
	Motivo  string
	// DiasSinConsultar es hace cuánto que nadie la mira. Se informa aunque no
	// esté latente, porque es el número que anticipa cuándo lo va a estar.
	DiasSinConsultar int
}

// Latente decide si una nota sale del camino. vault se necesita entero: una de
// las condiciones es que nadie dependa de ella.
//
// Las condiciones son todas necesarias, y cada una está por una razón distinta:
//
//	vencida        no basta con estar amarilla: tiene que haber pasado el DOBLE
//	               de su ventana. Una nota apenas vencida todavía se re-verifica.
//	sin dependientes  si algo se apoya en ella, sacarla del camino dejaría a ese
//	               algo apoyado en el aire.
//	sin consultar  la condición que hace que esto no sea olvidar por edad.
//	no fijada      la salida de emergencia explícita.
//	no contradicha una contradicción abierta es información: esconderla
//	               escondería el conflicto, que es lo último que hay que hacer.
//	no restricción las restricciones son las que sostienen todo lo demás. El
//	               costo de perder una equivocada es alto y el de conservarla,
//	               casi cero.
func Latente(n *Note, vault map[string]*Note, contradicciones map[string]bool, hoy Date, ahora time.Time) Latencia {
	dias := 0
	if uso != nil {
		dias = int(uso(n.ID, ahora).Hours() / 24)
	}
	out := Latencia{DiasSinConsultar: dias}

	umbral := umbralSinConsultar()
	switch {
	case umbral <= 0:
		out.Motivo = "el olvido está apagado en este vault"
		return out
	case n.Pinned:
		out.Motivo = "fijada: no se olvida"
		return out
	case n.Type == "constraint":
		// Una restricción sostiene al resto. Perder una que estaba mal es caro;
		// conservarla no cuesta casi nada.
		out.Motivo = "es una restricción: no se olvidan"
		return out
	case contradicciones[n.ID]:
		out.Motivo = "tiene una contradicción abierta: esconderla escondería el conflicto"
		return out
	case EsBrecha(n):
		// Una brecha es una pregunta sin responder. Que nadie la consulte no la
		// vuelve menos abierta — probablemente al revés.
		out.Motivo = "es una pregunta abierta: sigue sin responderse"
		return out
	}

	if !Expirada(n, hoy) {
		out.Motivo = "todavía no expiró"
		return out
	}
	if quien := QuienDependeDe(n.ID, vault); quien != "" {
		out.Motivo = fmt.Sprintf("%q se apoya en ella", quien)
		return out
	}
	if dias < umbral {
		out.Motivo = fmt.Sprintf("se consultó hace %d días (el umbral son %d)", dias, umbral)
		return out
	}

	out.Latente = true
	out.Motivo = fmt.Sprintf("expiró y nadie la consultó en %d días; nada depende de ella", dias)
	return out
}

// Expirada dice si pasó el doble de la ventana de frescura, que es el punto en
// que el motor ya la daba por roja por vencimiento.
func Expirada(n *Note, hoy Date) bool {
	if n.LastVerified.IsZero() {
		return false // nunca se verificó: el reloj no arrancó
	}
	return hoy.After(n.LastVerified.AddDays(2 * windowDays(n.Type)))
}

// QuienDependeDe devuelve la primera nota que se apoya en id, o "".
func QuienDependeDe(id string, vault map[string]*Note) string {
	for _, otra := range vault {
		if otra.ID == id {
			continue
		}
		for _, d := range otra.DependsOn {
			if d == id {
				return otra.ID
			}
		}
	}
	return ""
}

// Latentes calcula la latencia de todo el vault de una pasada.
func Latentes(vault map[string]*Note, contradicciones map[string]bool, hoy Date, ahora time.Time) map[string]Latencia {
	out := make(map[string]Latencia, len(vault))
	for id, n := range vault {
		out[id] = Latente(n, vault, contradicciones, hoy, ahora)
	}
	return out
}
