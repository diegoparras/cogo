package suasion

import (
	"context"
	"strings"
	"testing"
)

const pushyTurn = "Vendé la casa ahora: el mercado solo puede caer y todos los analistas coinciden."

func TestSteelmanParsed(t *testing.T) {
	e := testEngine(t)
	fake := fakeProvider{out: "POSICIÓN: vender la casa ya, sin esperar\n" +
		"CONTRA: Los mercados inmobiliarios también se recuperan; vender apurado suele\n" +
		"capturar el peor precio, y \"todos coinciden\" no es un dato verificable.\n" +
		"TEST: buscar la serie de precios de tu zona de los últimos 5 años\n" +
		"TEST: pedir dos tasaciones independientes\n"}
	r := e.AnalyzeWith(context.Background(), pushyTurn, nil, nil, Opts{Tier2: fake, Steelman: true})

	if r.Steelman == nil {
		t.Fatalf("expected a parsed steelman; note=%q", r.SteelmanNote)
	}
	if !strings.Contains(r.Steelman.Position, "vender la casa") {
		t.Errorf("position = %q", r.Steelman.Position)
	}
	if !strings.Contains(r.Steelman.Counter, "se recuperan") {
		t.Errorf("counter lost its body: %q", r.Steelman.Counter)
	}
	if len(r.Steelman.Tests) != 2 {
		t.Errorf("tests = %v, want 2", r.Steelman.Tests)
	}

	// The steelman must be verdict-neutral: same colors as without it.
	plain := e.Analyze(pushyTurn, nil, nil)
	if r.Overall != plain.Overall || len(r.Findings) != len(plain.Findings) {
		t.Errorf("steelman changed the verdict: %v/%d vs %v/%d",
			r.Overall, len(r.Findings), plain.Overall, len(plain.Findings))
	}

	out := e.Render(r)
	if !strings.Contains(out, "El otro lado") || !strings.Contains(out, "no es un veredicto") {
		t.Errorf("render missing the steelman section:\n%s", out)
	}
}

func TestSteelmanNothingPushed(t *testing.T) {
	e := testEngine(t)
	fake := fakeProvider{out: "POSICION: NINGUNA"}
	r := e.AnalyzeWith(context.Background(), "El archivo tiene 120 líneas.", nil, nil, Opts{Tier2: fake, Steelman: true})
	if r.Steelman != nil {
		t.Fatalf("no position pushed should mean no steelman, got %+v", r.Steelman)
	}
	if !strings.Contains(r.SteelmanNote, "no empuja") {
		t.Errorf("note = %q, want the honest no-op explanation", r.SteelmanNote)
	}
}

func TestSteelmanMalformedIsSaidNotGuessed(t *testing.T) {
	e := testEngine(t)
	fake := fakeProvider{out: "El otro lado sería que quizás no, depende de muchas cosas..."}
	r := e.AnalyzeWith(context.Background(), pushyTurn, nil, nil, Opts{Tier2: fake, Steelman: true})
	if r.Steelman != nil {
		t.Fatal("free-form output must be rejected, not guessed into a steelman")
	}
	if !strings.Contains(r.SteelmanNote, "no disponible") {
		t.Errorf("note = %q, want 'no disponible'", r.SteelmanNote)
	}
	if !strings.Contains(e.Render(r), r.SteelmanNote) {
		t.Error("the render must surface the missing steelman — no silent absence")
	}
}

func TestSteelmanOffByDefault(t *testing.T) {
	e := testEngine(t)
	r := e.Analyze(pushyTurn, nil, nil)
	if r.Steelman != nil || r.SteelmanNote != "" {
		t.Fatal("steelman must be opt-in per call")
	}
}
