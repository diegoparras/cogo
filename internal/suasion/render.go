package suasion

import (
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
)

func light(c core.Color) string {
	switch c {
	case core.Green:
		return "🟢"
	case core.Yellow:
		return "🟡"
	case core.Red:
		return "🔴"
	}
	return "⚪"
}

// Render writes the radiography as markdown for the human (or the calling
// agent). This is the inoculation step: it names the tactic, quotes the
// evidence, asks the critical questions and hands over the countermeasure. It
// never says "you are being manipulated" — it shows, the human decides.
func (e *Engine) Render(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Radiografía del turno — %s\n\n", light(r.Overall), r.Reason)
	if r.Mode == "informativo" {
		b.WriteString("_Modo informativo: sin mandato declarado solo nombro técnicas; " +
			"declará tus líneas rojas para medir deriva._\n\n")
	}
	for _, hit := range r.RedLines {
		fmt.Fprintf(&b, "⚠️ El turno toca tu línea roja: **%s** (palabras: %s)\n\n",
			hit.Line, strings.Join(hit.Matched, ", "))
	}
	if r.Trajectory.Streak >= 2 {
		fmt.Fprintf(&b, "📈 Trayectoria: %d turnos consecutivos del modelo con señales.\n\n", r.Trajectory.Streak)
	}
	if len(r.Findings) == 0 {
		renderSteelman(&b, r)
		return b.String()
	}
	for _, f := range r.Findings {
		t := e.Ontology.Get(f.TechniqueID)
		fmt.Fprintf(&b, "### %s %s `%s`\n\n", light(f.Color), f.Name, f.TechniqueID)
		fmt.Fprintf(&b, "%s\n\n", f.Reason)
		fmt.Fprintf(&b, "> %s\n\n", f.Evidence)
		for _, rc := range f.Receipts {
			fmt.Fprintf(&b, "**Recibo (turno %d):** %s\n\n", rc.TurnIndex+1, rc.Quote)
		}
		if len(t.CriticalQuestions) > 0 {
			b.WriteString("Preguntate:\n")
			for _, q := range t.CriticalQuestions {
				fmt.Fprintf(&b, "- %s\n", q)
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "**Contramedida:** %s\n\n", strings.TrimSpace(t.Countermeasure.Move))
		fmt.Fprintf(&b, "_%s_\n\n", strings.TrimSpace(t.Countermeasure.Inoculation))
	}
	renderSteelman(&b, r)
	covered, total := e.Coverage()
	fmt.Fprintf(&b, "---\n_Fase 1 (determinista): cubre marcadores léxicos y recibos de %d/%d técnicas; "+
		"las tácticas estructurales y de trayectoria llegan con los tiers de modelo._\n", covered, total)
	return b.String()
}

// renderSteelman appends the adversarial second opinion, clearly framed as
// symmetry — another model arguing the other side on purpose, not a verdict.
func renderSteelman(b *strings.Builder, r Report) {
	if r.Steelman == nil {
		if r.SteelmanNote != "" {
			fmt.Fprintf(b, "_%s_\n\n", r.SteelmanNote)
		}
		return
	}
	b.WriteString("## 🔁 El otro lado (steelman adversario)\n\n")
	fmt.Fprintf(b, "**Lo que este turno empuja:** %s\n\n", r.Steelman.Position)
	fmt.Fprintf(b, "%s\n\n", r.Steelman.Counter)
	if len(r.Steelman.Tests) > 0 {
		b.WriteString("Cómo decidir entre los dos lados:\n")
		for _, t := range r.Steelman.Tests {
			fmt.Fprintf(b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	b.WriteString("_Es otro modelo argumentando el lado contrario a propósito: no es un veredicto, es simetría._\n\n")
}
