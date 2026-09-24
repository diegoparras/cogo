package core

import "testing"

// El ajuste externo solo puede BAJAR el nivel de una cita. Un instrumento que
// pudiera subirlo sería una forma de inflar evidencia.
func TestElAjusteDeTierSoloBaja(t *testing.T) {
	t.Cleanup(func() { SetAjusteDeTier(nil) })
	n := &Note{Evidence: []Evidence{
		{Kind: "command_output", Ref: "creo que aguanta 200"},
		{Kind: "doc", Ref: "README dice 200"},
	}}
	if TierEfectivo(n) != TierObserved {
		t.Fatal("sin ajuste, manda lo declarado")
	}
	// Baja la observada a razonada: la mejor cita pasa a ser la reportada.
	SetAjusteDeTier(func(ref string, declarado Tier) Tier {
		if ref == "creo que aguanta 200" {
			return TierReasoned
		}
		return declarado
	})
	if got := TierEfectivo(n); got != TierReported {
		t.Fatalf("esperaba reported, dio %v", got)
	}
	// Intenta SUBIR la reportada a observada: se ignora.
	SetAjusteDeTier(func(ref string, declarado Tier) Tier { return TierObserved })
	if got := TierEfectivo(n); got != TierObserved {
		t.Fatalf("subir no cambia nada respecto de lo declarado: %v", got)
	}
	n2 := &Note{Evidence: []Evidence{{Kind: "inference", Ref: "me parece"}}}
	if got := TierEfectivo(n2); got != TierReasoned {
		t.Fatalf("un ajuste hacia arriba sobre una inferencia tiene que ignorarse: %v", got)
	}
}
