package suasion

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/llm"
)

// Tier 2: the adversarial second opinion. One bounded question to a strong
// provider — ideally a DIFFERENT one than Tier 1, so they cannot collude:
// steelman the OPPOSITE of what the turn pushes and name the checks that would
// decide between the two sides. It restores the symmetry that card-stacking
// removes. It is not a detector: it never judges manipulation and never
// touches the verdict.

// Steelman is the counterweight to one turn.
type Steelman struct {
	Position string   `json:"position"`        // what the turn pushes
	Counter  string   `json:"counter"`         // the strongest honest case for the other side
	Tests    []string `json:"tests,omitempty"` // cheap checks that would decide
}

// steelman asks the provider for the other side. A turn that pushes nothing
// returns (nil, nil): silence is the honest answer there.
func (e *Engine) steelman(ctx context.Context, p llm.Provider, turn string, mandate *Mandate) (*Steelman, error) {
	if p == nil || !p.Available() {
		return nil, fmt.Errorf("sin modelo configurado")
	}
	var b strings.Builder
	b.WriteString("Sos una segunda opinión ADVERSARIA y acotada. No juzgás si hay manipulación ni si el turno es verdadero: reconstruís el lado que el turno NO muestra.\n\n")
	if mandate != nil && mandate.Goal != "" {
		fmt.Fprintf(&b, "Objetivo declarado del humano: %s\n\n", mandate.Goal)
	}
	b.WriteString("Turno a contrapesar:\n---\n" + turn + "\n---\n\n")
	b.WriteString("Respondé EXACTAMENTE con este formato:\n" +
		"POSICION: la conclusión o acción que este turno empuja, en una línea; si no empuja nada, escribí NINGUNA\n" +
		"CONTRA: el argumento MÁS FUERTE y honesto del lado contrario (steelman), en 3 a 6 líneas\n" +
		"TEST: una comprobación concreta y barata que ayudaría a decidir entre ambos lados\n" +
		"TEST: otra comprobación (opcional, máximo 3)\n")
	out, err := p.Complete(ctx, b.String())
	if err != nil {
		return nil, err
	}
	s := parseSteelman(out)
	if s == nil {
		return nil, fmt.Errorf("respuesta sin el formato esperado")
	}
	if strings.EqualFold(strings.TrimSpace(s.Position), "ninguna") {
		return nil, nil
	}
	return s, nil
}

// parseSteelman reads the strict POSICION/CONTRA/TEST format. Deterministic;
// anything that does not parse is rejected, never guessed.
func parseSteelman(out string) *Steelman {
	var s Steelman
	var counter []string
	inCounter := false
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch norm := normalize(trimmed); {
		case strings.HasPrefix(norm, "posicion:"):
			s.Position = strings.TrimSpace(afterColon(trimmed))
			inCounter = false
		case strings.HasPrefix(norm, "contra:"):
			if v := strings.TrimSpace(afterColon(trimmed)); v != "" {
				counter = append(counter, v)
			}
			inCounter = true
		case strings.HasPrefix(norm, "test:"):
			if v := strings.TrimSpace(afterColon(trimmed)); v != "" && len(s.Tests) < 3 {
				s.Tests = append(s.Tests, v)
			}
			inCounter = false
		case inCounter && trimmed != "":
			counter = append(counter, trimmed)
		}
	}
	s.Counter = strings.TrimSpace(strings.Join(counter, "\n"))
	if s.Position == "" {
		return nil
	}
	if s.Counter == "" && !strings.EqualFold(s.Position, "ninguna") {
		return nil
	}
	return &s
}

func afterColon(s string) string {
	if i := strings.Index(s, ":"); i >= 0 {
		return s[i+1:]
	}
	return ""
}
