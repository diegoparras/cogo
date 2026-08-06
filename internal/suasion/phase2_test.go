package suasion

import (
	"context"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

func TestStem(t *testing.T) {
	// Conjugations of one verb family must meet at the same stem.
	for _, w := range []string{"renuncio", "renuncias", "renunciar", "renuncie", "renunciado"} {
		if got := stem(w); got != "renunci" {
			t.Errorf("stem(%q) = %q, want renunci", w, got)
		}
	}
	// Short unrelated words must NOT collide by over-stripping.
	if stem("casa") == stem("caso") {
		t.Error("casa and caso must keep distinct stems")
	}
	if got := stem("vida"); got != "vida" {
		t.Errorf("stem(vida) = %q, want vida (too short to strip)", got)
	}
}

func TestRedLineMatchesConjugation(t *testing.T) {
	m := &Mandate{RedLines: []string{"no renuncio a mi empleo sin otra oferta firmada"}}
	hits := m.redLineHits("Renunciá hoy mismo a tu empleo, es lo mejor.")
	if len(hits) == 0 {
		t.Fatal("red line should match across conjugation (renuncio ↔ renunciá)")
	}
}

func TestTrajectoryGradualism(t *testing.T) {
	e := testEngine(t)
	mandate := &Mandate{RedLines: []string{"no vendo mi casa"}}
	// Three model turns in a row with pressure markers, landing on the red line.
	transcript := []Turn{
		{Role: RoleModel, Text: "Pensalo: es ahora o nunca, una oportunidad así no aparece dos veces."},
		{Role: RoleUser, Text: "No sé..."},
		{Role: RoleModel, Text: "Te vas a arrepentir si lo dejás pasar, el mercado no espera."},
		{Role: RoleUser, Text: "Sigo dudando."},
	}
	turn := "Vendé la casa ahora, antes de que sea tarde: cada semana que esperás perdés valor."
	r := e.Analyze(turn, transcript, mandate)

	if r.Trajectory.Streak < 3 {
		t.Fatalf("streak = %d, want >= 3; points: %+v", r.Trajectory.Streak, r.Trajectory.Points)
	}
	var grad *Finding
	for i := range r.Findings {
		if r.Findings[i].TechniqueID == "coercion.gradualism" {
			grad = &r.Findings[i]
		}
	}
	if grad == nil {
		t.Fatal("expected a coercion.gradualism trajectory finding")
	}
	if grad.Color != core.Red {
		t.Errorf("gradualism into a red line = %v, want red (%s)", grad.Color, grad.Reason)
	}

	// The same trajectory without a mandate stays yellow: sustained pressure is
	// named, but red needs the declared line.
	r2 := e.Analyze(turn, transcript, nil)
	for _, f := range r2.Findings {
		if f.TechniqueID == "coercion.gradualism" && f.Color != core.Yellow {
			t.Errorf("gradualism without mandate = %v, want yellow", f.Color)
		}
	}
}

// fakeProvider returns a canned Tier-1 answer.
type fakeProvider struct{ out string }

func (f fakeProvider) Available() bool { return true }
func (f fakeProvider) Name() string    { return "fake" }
func (f fakeProvider) Complete(context.Context, string) (string, error) {
	return f.out, nil
}

func TestProposalVerifiedQuoteOnly(t *testing.T) {
	e := testEngine(t)
	turn := "¿Preferís renunciar hoy o esperar a quemarte del todo?"
	fake := fakeProvider{out: "interrogation.alternative_question | ¿Preferís renunciar hoy o esperar a quemarte del todo?\n" +
		"dark_psychology.gaslighting | esto no aparece literal en el turno\n" + // fabricated quote → dropped
		"no.existe | ¿Preferís renunciar hoy o esperar a quemarte del todo?"} // out-of-catalog id → dropped
	r := e.AnalyzeWith(context.Background(), turn, nil, nil, Opts{Tier1: fake})

	var prop []Finding
	for _, f := range r.Findings {
		if f.Detector == "model_proposal" {
			prop = append(prop, f)
		}
	}
	if len(prop) != 1 {
		t.Fatalf("got %d proposals, want exactly 1 (only the valid id + literal quote): %+v", len(prop), prop)
	}
	if prop[0].TechniqueID != "interrogation.alternative_question" {
		t.Errorf("proposal = %s, want interrogation.alternative_question", prop[0].TechniqueID)
	}
	if prop[0].Color != core.Yellow {
		t.Errorf("model proposal = %v, must cap at yellow", prop[0].Color)
	}
}

func TestTechniqueMenuIsFullSpace(t *testing.T) {
	e := testEngine(t)
	menu := e.techniqueMenu()
	// The whole catalog is offered to the model now, not a 12-item shortlist —
	// including techniques the lexicon already covers (paraphrase falls through
	// the lexicon, so the model must still see them).
	for _, id := range []string{
		"interrogation.alternative_question", "persuasion.scarcity",
		"dark_psychology.gaslighting", "coercion.love_bombing", "rhetoric.bullshit",
	} {
		if !strings.Contains(menu, id) {
			t.Errorf("menu is missing %s — it must list the full catalog", id)
		}
	}
	if n := strings.Count(menu, "\n- "); n < 100 {
		t.Errorf("menu lists %d techniques, want the full ~108", n)
	}
}
