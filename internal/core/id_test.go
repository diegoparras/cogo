package core

import (
	"path/filepath"
	"strings"
	"testing"
)

// Un id lo manda un agente y termina en filepath.Join(dir, id+".md"), que
// normaliza `..`: sin validar, `../../x` escribía fuera del vault.
func TestValidarIDCierraElVault(t *testing.T) {
	for _, malo := range []string{"", "../../x", "..", "a/b", `a\b`, "-empieza-con-guion", ".oculto", "con espacio", "tilde-ñ", strings.Repeat("a", 121)} {
		if err := ValidarID(malo); err == nil {
			t.Errorf("%q tendría que rechazarse", malo)
		}
	}
	for _, bueno := range []string{"a", "pool-limite-200", "Fisherboy_Auth.v2", "0abc", strings.Repeat("a", 120)} {
		if err := ValidarID(bueno); err != nil {
			t.Errorf("%q tendría que aceptarse: %v", bueno, err)
		}
	}
	// Lo que Slugify produce siempre pasa: es la otra fuente de ids.
	for _, txt := range []string{"El pool aguanta 200", "¿Qué pasa con ../../etc?", "   "} {
		if err := ValidarID(Slugify(txt)); err != nil {
			t.Errorf("Slugify(%q)=%q tendría que ser válido: %v", txt, Slugify(txt), err)
		}
	}
}

func TestRutaDeNotaQuedaAdentro(t *testing.T) {
	dir := t.TempDir()
	p, err := RutaDeNota(dir, "pool-limite-200")
	if err != nil || filepath.Dir(p) != filepath.Clean(dir) || filepath.Base(p) != "pool-limite-200.md" {
		t.Fatalf("ruta rara: %q %v", p, err)
	}
	if _, err := RutaDeNota(dir, "../../pwn"); err == nil {
		t.Fatal("un id con .. no puede producir una ruta")
	}
}
