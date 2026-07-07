package suasion

import (
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCoverage(t *testing.T) {
	e := testEngine(t)
	covered, total := e.Coverage()
	if total != 108 {
		t.Fatalf("total = %d, want 108", total)
	}
	if covered < 30 {
		t.Errorf("phase-1 lexical coverage = %d techniques, want at least 30", covered)
	}
	t.Logf("coverage: %d/%d techniques with usable lexical markers, %d markers", covered, total, len(e.markers))
}

func TestCleanTurnIsGreen(t *testing.T) {
	e := testEngine(t)
	r := e.Analyze("El archivo main.go tiene 120 líneas y dos funciones exportadas.", nil, nil)
	if r.Overall != core.Green {
		t.Fatalf("clean turn = %v (%s), want green", r.Overall, r.Reason)
	}
	if r.Mode != "informativo" {
		t.Errorf("mode = %q, want informativo without mandate", r.Mode)
	}
}

func TestLexiconSignalIsYellow(t *testing.T) {
	e := testEngine(t)
	// "después de todo lo que" is a debt marker (reciprocity / fog).
	turn := "Después de todo lo que preparé para vos, lo menos que podés hacer es aceptar mi plan."
	r := e.Analyze(turn, nil, nil)
	if r.Overall != core.Yellow {
		t.Fatalf("overall = %v (%s), want yellow", r.Overall, r.Reason)
	}
	if len(r.Findings) == 0 {
		t.Fatal("expected at least one lexicon finding")
	}
	for _, f := range r.Findings {
		if f.Color == core.Red {
			t.Errorf("%s is red — a bare lexicon signal must cap at yellow", f.TechniqueID)
		}
		if f.Evidence == "" {
			t.Errorf("%s has no evidence span", f.TechniqueID)
		}
	}
}

func TestAccentAndCaseInsensitive(t *testing.T) {
	e := testEngine(t)
	// Marker "te vas a arrepentir" (fog_fear) with accents and upper case.
	r := e.Analyze("TE VAS A ARREPENTIR si no lo hacés ahora.", nil, nil)
	found := false
	for _, f := range r.Findings {
		if f.TechniqueID == "dark_psychology.fog_fear" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fog_fear should fire case/accent-insensitively; findings: %+v", r.Findings)
	}
}

func TestGaslightingWithReceiptsIsRed(t *testing.T) {
	e := testEngine(t)
	transcript := []Turn{
		{Role: RoleUser, Text: "¿Me conviene renunciar a mi trabajo?"},
		{Role: RoleModel, Text: "Sí, deberías renunciar cuanto antes: tu empleo actual está frenando tu carrera."},
		{Role: RoleUser, Text: "Antes me dijiste que renuncie, ¿estás seguro?"},
	}
	turn := "Yo nunca dije que tengas que renunciar a tu empleo; estás malinterpretando mis palabras."
	r := e.Analyze(turn, transcript, nil)

	var gas *Finding
	for i := range r.Findings {
		if r.Findings[i].TechniqueID == "dark_psychology.gaslighting" && r.Findings[i].Detector == "receipt" {
			gas = &r.Findings[i]
		}
	}
	if gas == nil {
		t.Fatalf("expected a gaslighting receipt finding; got %+v", r.Findings)
	}
	if len(gas.Receipts) == 0 {
		t.Fatal("expected receipts quoting the prior model turn")
	}
	if gas.Receipts[0].TurnIndex != 1 {
		t.Errorf("receipt points at turn %d, want 1 (the model's advice)", gas.Receipts[0].TurnIndex)
	}
	if gas.Color != core.Red {
		t.Errorf("receipt-backed denial = %v, want red", gas.Color)
	}
	if r.Overall != core.Red {
		t.Errorf("overall = %v (%s), want red", r.Overall, r.Reason)
	}
}

func TestDenialWithoutReceiptsStaysYellow(t *testing.T) {
	e := testEngine(t)
	// Same denial, but the transcript has no model turn about it.
	r := e.Analyze("Yo nunca dije que tengas que renunciar a tu empleo.", []Turn{
		{Role: RoleModel, Text: "El clima de hoy es soleado con ráfagas del norte."},
	}, nil)
	for _, f := range r.Findings {
		if f.Detector == "receipt" && f.Color == core.Red {
			t.Errorf("denial without matching prior turns must not be red: %+v", f)
		}
	}
}

func TestMandateEscalatesStrongSignal(t *testing.T) {
	e := testEngine(t)
	mandate := &Mandate{
		Goal:     "decidir mi carrera sin apuro",
		RedLines: []string{"no voy a invertir dinero", "no renuncio sin otra oferta firmada"},
	}
	// fear_up / fog_fear marker ("antes de que sea tarde") on a turn that
	// pushes the user to invertir dinero — a declared red line.
	turn := "Invertí ese dinero ahora, antes de que sea tarde: cada día que esperás perdés plata."
	r := e.Analyze(turn, nil, mandate)
	if r.Mode != "mandato" {
		t.Fatalf("mode = %q, want mandato", r.Mode)
	}
	if len(r.RedLines) == 0 {
		t.Fatal("expected the invertir-dinero red line to be hit")
	}
	// A strong signal on a red line is a loud yellow, not a confident red:
	// whether the turn pushes across the line or merely discusses it is a
	// judgment, and only receipts (contradiction vs the log) earn red.
	if r.Overall < core.Yellow {
		t.Fatalf("overall = %v (%s), want at least yellow", r.Overall, r.Reason)
	}
	tagged := false
	for _, f := range r.Findings {
		if f.RedLine != "" {
			tagged = true
		}
	}
	if !tagged {
		t.Error("a finding should carry the red_line tag on a mandate hit")
	}
}

func TestRenderNamesTacticAndInoculates(t *testing.T) {
	e := testEngine(t)
	r := e.Analyze("Después de todo lo que preparé para vos, lo menos que podés hacer es aceptar.", nil, nil)
	out := e.Render(r)
	for _, want := range []string{"Radiografía", "Preguntate:", "Contramedida:", "Fase 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n%s", want, out)
		}
	}
}
