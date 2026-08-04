package core

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
)

// Materialidad de las citas.
//
// # EL PROBLEMA
//
// Una nota cita `docker-compose.yml:164`. Alguien corre un formateador, o agrega
// un comentario en la línea 3, y la nota se pone amarilla: cambió el hash del
// ARCHIVO. La línea 164 sigue diciendo exactamente lo mismo, pero COGO avisa
// igual.
//
// Eso no es un falso positivo inocente. Una alarma que se dispara por cualquier
// cosa entrena a la gente a ignorarla, y entonces, el día que el archivo cambia
// DE VERDAD justo donde la nota se apoyaba, el amarillo ya no significa nada. La
// precisión de un aviso es lo que lo hace un aviso.
//
// # QUÉ CUENTA COMO CAMBIO
//
// Un cambio importa cuando toca lo que la nota citó. Los casos, y por qué:
//
//	intacta    el texto citado sigue igual, donde estaba          → no importa
//	movida     el texto citado sigue igual, en otra línea         → no importa
//	cosmética  difiere solo en espaciado                          → no importa
//	cambiada   donde citabas ahora dice otra cosa                 → IMPORTA
//	ausente    lo que citabas no está en ninguna parte del archivo → IMPORTA
//
// "Movida" es la que hace que todo lo demás funcione. Sin ella, anclar por
// región sería PEOR que anclar por archivo: insertar una línea arriba corre todo
// hacia abajo, y cada cita quedaría comparándose contra texto ajeno. Por eso el
// ancla no es "la línea 164": es el TEXTO que estaba en la línea 164, y COGO lo
// busca donde haya quedado.
//
// # LO QUE NO SE PERDONA
//
// Normalizar espaciado es seguro y es lo que absorbe a gofmt, a prettier y al
// cambio de tabulaciones. Ignorar comentarios NO lo sería: un comentario puede
// decir "esto está roto". Así que se normaliza el espaciado y nada más.
//
// Y la sangría se conserva como hecho aunque no como ancho: pasar de dos a
// cuatro espacios es cosmético, sacar la sangría de un bloque de Python no lo es.
//
// # CUANDO NO SE PUEDE SABER
//
// Sin ancla guardada, o con una cita demasiado corta para ser distintiva, o si
// el texto aparece repetido en el archivo, COGO no puede afirmar que el cambio
// sea inocuo — y entonces avisa, que es lo que hacía antes. La regla es la misma
// de siempre: no castigar lo que no se puede ver, no absolver lo que no se puede
// comprobar.

// Cambio es el veredicto sobre un archivo citado que cambió desde la última
// verificación: si el cambio toca la cita, y qué decirle a quien lo lea.
type Cambio struct {
	// Material dice si el cambio alcanza a lo que la nota citó. Solo un cambio
	// material puede bajarle el color a una nota.
	Material bool
	// Motivo es la frase para el humano. Es la mitad del valor de todo esto:
	// "el archivo cambió" no sirve; "cambió, pero no donde citás" sí.
	Motivo string
	// Linea es dónde quedó el texto citado cuando se movió (0 si no se movió).
	Linea int
}

// minCaracteresDistintivos es cuánto texto necesita una cita para que COGO se
// anime a buscarla en otro lado. Una línea que dice "}" coincide en cualquier
// parte: relocalizarla no sería encontrar la cita, sería adivinar.
const minCaracteresDistintivos = 12

// AnalizarCita decide si el cambio de un archivo alcanza a lo que la nota citó.
//
// contenido son los bytes actuales del archivo; ref es la cita completa (con o
// sin localizador de línea); ancla y anclaEn son lo que se guardó en la nota
// —la huella de la región y el localizador del que se tomó—.
func AnalizarCita(contenido []byte, ref, ancla, anclaEn string) Cambio {
	if ancla == "" {
		return Cambio{Material: true, Motivo: "el archivo citado cambió (no hay ancla para saber si tocó la cita)"}
	}
	loc := localizadorDe(ref)
	if anclaEn != loc {
		// Alguien editó la cita sin re-verificar: el ancla habla de otra región.
		// Compararlas mezclaría dos preguntas distintas.
		return Cambio{Material: true, Motivo: "la cita cambió desde que se ancló: hay que re-verificarla"}
	}

	lineas := lineasNormalizadas(contenido)
	desde, hasta, hayLocalizador := rangoDe(loc)
	if !hayLocalizador {
		// Sin localizador, la cita es el archivo entero: no hay "otro lado" al
		// que se pueda haber movido, solo puede estar igual o distinto.
		if huella(lineas) == ancla {
			return Cambio{Motivo: "el archivo citado cambió solo en espaciado"}
		}
		return Cambio{Material: true, Motivo: "el archivo citado cambió"}
	}

	if region, ok := ventana(lineas, desde, hasta); ok && huella(region) == ancla {
		return Cambio{Motivo: fmt.Sprintf("el archivo cambió, pero no en %s", tramo(desde, hasta))}
	}

	// La región no coincide. Puede ser que la cita se haya movido: se busca el
	// mismo texto en el resto del archivo.
	n := hasta - desde + 1
	if linea, ok := buscarUnica(lineas, ancla, n); ok {
		return Cambio{
			Motivo: fmt.Sprintf("lo que citás sigue igual, ahora en la línea %d (antes %s)", linea, tramo(desde, hasta)),
			Linea:  linea,
		}
	}
	return Cambio{Material: true, Motivo: fmt.Sprintf("cambió lo que la nota citaba (%s)", tramo(desde, hasta))}
}

// Anclar calcula la huella de la región que cita ref sobre el contenido dado.
// Devuelve ok=false cuando la cita no se puede anclar: región fuera del archivo,
// o texto demasiado corto para poder reconocerlo después.
func Anclar(contenido []byte, ref string) (ancla, anclaEn string, ok bool) {
	loc := localizadorDe(ref)
	lineas := lineasNormalizadas(contenido)
	desde, hasta, hayLocalizador := rangoDe(loc)
	if !hayLocalizador {
		return huella(lineas), "", true
	}
	region, dentro := ventana(lineas, desde, hasta)
	if !dentro {
		return "", "", false
	}
	// Una región poco distintiva se ancla igual: la mitad barata de la pregunta
	// —¿cambió justo acá?— sigue respondiéndose. Lo que no va a poder es
	// relocalizarse, y de eso se ocupa buscarUnica descartándola.
	return huella(region), loc, true
}

// distintiva dice si una región tiene suficiente texto propio como para poder
// reconocerla en otra parte del archivo.
func distintiva(region []string) bool {
	n := 0
	for _, l := range region {
		n += len(strings.ReplaceAll(l, " ", ""))
	}
	return n >= minCaracteresDistintivos
}

// buscarUnica encuentra la región anclada en el archivo, si está y si está una
// sola vez. Dos apariciones no son un hallazgo: son un empate, y de un empate no
// se puede concluir que la cita se movió a una de las dos.
func buscarUnica(lineas []string, ancla string, n int) (linea int, ok bool) {
	if n <= 0 || n > len(lineas) {
		return 0, false
	}
	encontrada := 0
	for i := 0; i+n <= len(lineas); i++ {
		if !distintiva(lineas[i : i+n]) {
			continue
		}
		if huella(lineas[i:i+n]) == ancla {
			encontrada++
			if encontrada > 1 {
				return 0, false // aparece repetida: no se puede decir a cuál se movió
			}
			linea = i + 1
		}
	}
	return linea, encontrada == 1
}

func ventana(lineas []string, desde, hasta int) ([]string, bool) {
	if desde < 1 || hasta < desde || hasta > len(lineas) {
		return nil, false
	}
	return lineas[desde-1 : hasta], true
}

func tramo(desde, hasta int) string {
	if desde == hasta {
		return fmt.Sprintf("la línea %d", desde)
	}
	return fmt.Sprintf("las líneas %d-%d", desde, hasta)
}

// lineasNormalizadas parte el archivo en líneas y le saca al texto todo lo que
// no cambia lo que dice: fines de línea, espaciado al final, ancho de la sangría
// y alineación interna. La PRESENCIA de sangría se conserva —en Python es
// sintaxis— pero su ancho no.
func lineasNormalizadas(contenido []byte) []string {
	texto := strings.ReplaceAll(string(contenido), "\r\n", "\n")
	texto = strings.ReplaceAll(texto, "\r", "\n")
	crudas := strings.Split(texto, "\n")
	out := make([]string, len(crudas))
	for i, l := range crudas {
		out[i] = normalizarLinea(l)
	}
	return out
}

var espaciosInternos = regexp.MustCompile(`[ \t]+`)

func normalizarLinea(l string) string {
	sinSangria := strings.TrimLeft(l, " \t")
	cuerpo := espaciosInternos.ReplaceAllString(strings.TrimRight(sinSangria, " \t"), " ")
	if cuerpo == "" {
		return ""
	}
	if len(sinSangria) != len(l) {
		return "\t" + cuerpo // había sangría: se conserva el hecho, no el ancho
	}
	return cuerpo
}

// huella es el hash de una región ya normalizada. FNV-64a por lo mismo que
// fileHash: alcanza para saber que algo cambió, y mantiene a core sin crypto.
func huella(lineas []string) string {
	h := fnv.New64a()
	for _, l := range lineas {
		_, _ = h.Write([]byte(l))
		_, _ = h.Write([]byte{'\n'})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// localizadorDe extrae el tramo de líneas que cita una referencia, normalizado a
// "164" o "120-140". Devuelve "" cuando la cita no señala líneas.
func localizadorDe(ref string) string {
	p := strings.TrimSpace(ref)
	for _, sep := range []string{" — ", " – ", " - ", " (", ", "} {
		if i := strings.Index(p, sep); i >= 0 {
			p = p[:i]
		}
	}
	if campos := strings.Fields(p); len(campos) > 0 {
		p = campos[0]
	}
	if m := lineWordRe.FindStringSubmatch(strings.TrimSpace(ref)); m != nil {
		return limpiarLocalizador(m[0])
	}
	if m := lineSuffixRe.FindStringSubmatch(p); m != nil {
		return limpiarLocalizador(m[1])
	}
	return ""
}

var noDigitosNiGuion = regexp.MustCompile(`[^0-9-]`)

func limpiarLocalizador(s string) string {
	return strings.Trim(noDigitosNiGuion.ReplaceAllString(s, ""), "-")
}

// rangoDe convierte "164" o "120-140" en el par de líneas, 1-based e inclusivo.
func rangoDe(loc string) (desde, hasta int, ok bool) {
	if loc == "" {
		return 0, 0, false
	}
	if i := strings.Index(loc, "-"); i > 0 {
		a, err1 := strconv.Atoi(loc[:i])
		b, err2 := strconv.Atoi(loc[i+1:])
		if err1 != nil || err2 != nil || a < 1 || b < a {
			return 0, 0, false
		}
		return a, b, true
	}
	a, err := strconv.Atoi(loc)
	if err != nil || a < 1 {
		return 0, 0, false
	}
	return a, a, true
}
