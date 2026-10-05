package xray

import "testing"

func TestAnalyzeGapMeter(t *testing.T) {
	cases := []struct {
		text      string
		wantColor string
		wantEv    string
	}{
		{"Sin duda el servidor siempre responde en menos de 10ms.", "red", "none"}, // boosted + no basis
		{"Esta librería es claramente la mejor que existe.", "red", "none"},        // opinion (not falsifiable)
		{"Según la documentación oficial, el puerto por defecto es 8080.", "yellow", "reported"},
		{"Probablemente el bug está en el módulo de red del worker.", "yellow", "none"}, // hedged
		{"Verifiqué en el log que la conexión se cae a los 30 segundos.", "yellow", "observed"},
	}
	for _, c := range cases {
		r := Analyze(c.text)
		if len(r.Claims) != 1 {
			t.Fatalf("%q: expected 1 claim, got %d", c.text, len(r.Claims))
		}
		got := r.Claims[0]
		if got.Color != c.wantColor {
			t.Errorf("%q: color = %q, want %q (%s)", c.text, got.Color, c.wantColor, got.Reason)
		}
		if got.Evidence != c.wantEv {
			t.Errorf("%q: evidence = %q, want %q", c.text, got.Evidence, c.wantEv)
		}
	}

	// A red claim makes the whole answer red; empty text is ungraded.
	if Analyze("Sin duda es siempre así.").Overall != "red" {
		t.Error("a red claim should make the overall red")
	}
	if Analyze("").Overall != "ungraded" {
		t.Error("empty answer should be ungraded")
	}
}
