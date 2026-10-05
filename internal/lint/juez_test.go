package lint

import (
	"context"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/llm"
)

// Con un juez, lint propone contradicciones sin ningún modelo generativo, y
// respeta el umbral. Sin juez ni proveedor, no propone nada.
func TestElJuezProponeContradicciones(t *testing.T) {
	t.Cleanup(func() { SetJuezDeContradicciones(nil, nil) })
	vault := map[string]*core.Note{
		"pool-200": {ID: "pool-200", Project: "p", Body: "## Claim\nEl pool de conexiones aguanta 200 conexiones concurrentes."},
		"pool-50":  {ID: "pool-50", Project: "p", Body: "## Claim\nEl pool de conexiones satura a las 50 conexiones concurrentes."},
		"cron":     {ID: "cron", Project: "p", Body: "## Claim\nEl cron de limpieza corre a las tres de la mañana."},
	}
	hoy := core.NewDate(2026, 9, 23)
	if r := Run(context.Background(), vault, hoy, llm.Noop{}); r.JuezUsado || r.PairsChecked != 0 {
		t.Fatal("sin juez ni proveedor no se pregunta nada")
	}
	preguntados := 0
	SetJuezDeContradicciones(func(_ context.Context, a, b *core.Note) (float64, bool) {
		preguntados++
		if (a.ID == "pool-200" && b.ID == "pool-50") || (a.ID == "pool-50" && b.ID == "pool-200") {
			return 0.93, true
		}
		return 0.05, true
	}, func() float64 { return 0.8 })
	r := Run(context.Background(), vault, hoy, llm.Noop{})
	if !r.JuezUsado || r.LLMUsed {
		t.Fatalf("tiene que haber usado el juez y no el LLM: %+v", r)
	}
	var contradicciones int
	for _, is := range r.Issues {
		if is.Kind == "contradiction" {
			contradicciones++
			if is.IDs[0] != "pool-200" || is.IDs[1] != "pool-50" {
				t.Errorf("par inesperado: %v", is.IDs)
			}
		}
	}
	if contradicciones != 1 {
		t.Fatalf("esperaba 1 contradicción propuesta, hubo %d: %+v", contradicciones, r.Issues)
	}
	if preguntados == 0 {
		t.Fatal("el juez no fue consultado")
	}
}
