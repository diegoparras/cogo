package presencia

import (
	"fmt"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/lease"
)

// El aviso que se cuelga de un `pack` o de un `authorize`.
//
// # LA REGLA QUE LO HACE ÚTIL
//
// Un aviso que aparece siempre es un aviso que nadie lee. Este solo sale cuando
// hay algo que decir: otro agente, activo hace poco, en el mismo proyecto o con
// un permiso tomado. Si no se cumple, la respuesta sale como antes.
//
// Es la misma disciplina que la materialidad de las citas: la precisión de un
// aviso es lo que lo hace un aviso.

// Aviso arma el bloque en inglés —lo lee un agente, no un humano— o "" si no hay
// nada que avisar.
func Aviso(otros []Agente, leases []lease.Lease, yo string, ahora time.Time) string {
	var propios, ajenos []lease.Lease
	for _, l := range leases {
		if strings.TrimSpace(l.Holder) == strings.TrimSpace(yo) {
			propios = append(propios, l)
		} else {
			ajenos = append(ajenos, l)
		}
	}
	if len(otros) == 0 && len(ajenos) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n---\n## Somebody else is working here\n")

	// Los permisos primero: son la señal más fuerte, y la única que dice
	// "esto se está haciendo AHORA" en vez de "esto se hizo recién".
	for _, l := range ajenos {
		fmt.Fprintf(&b, "- **%s** holds the lease `%s` until %s", l.Holder, l.Name, l.Expires)
		if strings.TrimSpace(l.Note) != "" {
			fmt.Fprintf(&b, " — %q", l.Note)
		}
		b.WriteString("\n")
	}
	for _, a := range otros {
		verbo := "is reading here"
		if a.Escribiendo() {
			verbo = "is WRITING here"
		}
		fmt.Fprintf(&b, "- **%s** %s (%s)", a.Token, verbo, haceCuanto(a.Ultima, ahora))
		if len(a.Herramientas) > 0 {
			fmt.Fprintf(&b, " via %s", strings.Join(recorte(a.Herramientas, 4), ", "))
		}
		if len(a.Notas) > 0 {
			fmt.Fprintf(&b, " — on %s", strings.Join(recorte(a.Notas, 4), ", "))
		}
		b.WriteString("\n")
	}

	b.WriteString("\nBefore you write or run anything here: check `recall` for what changed, " +
		"and take a `lease` on whatever you're about to touch. If your work overlaps theirs, " +
		"say so to the human instead of racing them.\n")
	return b.String()
}

// Choque busca un permiso ajeno cuyo nombre aparezca en el texto de la acción.
//
// Es una coincidencia de nombre, no una inferencia: si alguien tomó el permiso
// "migrar-db" y la acción dice "migrar-db", eso no es una sospecha, es la misma
// cosa. Un criterio más flojo produciría bloqueos falsos, y un bloqueo falso en
// una herramienta de seguridad se paga con que la apaguen.
func Choque(accion string, leases []lease.Lease, yo string) (lease.Lease, bool) {
	txt := strings.ToLower(accion)
	for _, l := range leases {
		if strings.TrimSpace(l.Holder) == strings.TrimSpace(yo) {
			continue // el propio permiso no choca: para eso se tomó
		}
		n := strings.ToLower(strings.TrimSpace(l.Name))
		if n != "" && strings.Contains(txt, n) {
			return l, true
		}
	}
	return lease.Lease{}, false
}

func haceCuanto(t, ahora time.Time) string {
	d := ahora.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func recorte(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return append(append([]string{}, xs[:n]...), "…")
}
