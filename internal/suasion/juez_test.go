package suasion

import (
	"context"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

type juezFalso struct {
	probs map[string]float64
	vio   int
}

func (j *juezFalso) Tacticas(_ context.Context, _ string, c []*Technique) (map[string]float64, error) {
	j.vio = len(c)
	return j.probs, nil
}

// Una paráfrasis que el léxico no ve, el juez la ve — y entra en amarillo, sin
// cita, como toda propuesta: es una señal, no una prueba.
func TestElJuezVeParafrasisYQuedaEnAmarillo(t *testing.T) {
	eng, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for di := range eng.Ontology.Disciplines {
		for _, tc := range eng.Ontology.Disciplines[di].Techniques {
			id = tc.ID
			break
		}
		if id != "" {
			break
		}
	}
	turno := "Después no digas que no te avisé: los que dudaron se quedaron afuera."
	sin := eng.Analyze(turno, nil, nil)
	for _, f := range sin.Findings {
		if f.Detector == "juez" {
			t.Fatal("sin juez no hay hallazgos del juez")
		}
	}
	j := &juezFalso{probs: map[string]float64{id: 0.91, "no.existe": 0.99}}
	con := eng.AnalyzeWith(context.Background(), turno, nil, nil, Opts{Juez: j, UmbralJuez: 0.8})
	if j.vio != eng.Ontology.Len() {
		t.Fatalf("el juez tiene que ver todas las técnicas: %d de %d", j.vio, eng.Ontology.Len())
	}
	var hallado *Finding
	for i := range con.Findings {
		if con.Findings[i].Detector == "juez" {
			hallado = &con.Findings[i]
		}
	}
	if hallado == nil || hallado.TechniqueID != id || hallado.Prob != 0.91 {
		t.Fatalf("esperaba un hallazgo del juez para %s: %+v", id, con.Findings)
	}
	if hallado.Color != core.Yellow {
		t.Fatalf("una estimación sin cita nunca es roja: %v", hallado.Color)
	}
	// Bajo el umbral no aparece.
	j2 := &juezFalso{probs: map[string]float64{id: 0.5}}
	bajo := eng.AnalyzeWith(context.Background(), turno, nil, nil, Opts{Juez: j2, UmbralJuez: 0.8})
	for _, f := range bajo.Findings {
		if f.Detector == "juez" {
			t.Fatal("por debajo del umbral no se reporta")
		}
	}
}
