package suasion

import (
	"context"
	"sort"
)

// juzgar le pregunta al juez por TODAS las técnicas de la ontología sobre el
// turno, en una solicitud, y se queda con las que superan el umbral. Tope de
// cuatro, como las propuestas del modelo: un turno con doce tácticas no es
// más informativo que uno con cuatro, es ruido.
func (e *Engine) juzgar(ctx context.Context, j JuezDeTacticas, turn string, umbral float64) []Finding {
	if j == nil || e.Ontology == nil {
		return nil
	}
	if umbral <= 0 {
		umbral = 0.8
	}
	var candidatas []*Technique
	for di := range e.Ontology.Disciplines {
		for ti := range e.Ontology.Disciplines[di].Techniques {
			candidatas = append(candidatas, &e.Ontology.Disciplines[di].Techniques[ti])
		}
	}
	probs, err := j.Tacticas(ctx, turn, candidatas)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, t := range candidatas {
		p, ok := probs[t.ID]
		if !ok || p < umbral {
			continue
		}
		out = append(out, Finding{
			TechniqueID: t.ID, Name: t.Name, Axes: t.Axes, Severity: t.Severity,
			Detector: "juez", Prob: p,
			Evidence: "(sin cita: el juez estima sobre el turno entero)",
		})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Prob > out[b].Prob })
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}
