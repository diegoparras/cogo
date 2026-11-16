package atomico

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEscribirReemplazaSinDejarRastro(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "estado.json")
	if err := Escribir(p, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Escribir(p, []byte(`{"v":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != `{"v":2}` {
		t.Fatalf("contenido: %q %v", b, err)
	}
	entradas, _ := os.ReadDir(dir)
	if len(entradas) != 1 {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Fatalf("no puede quedar ningún temporal: %v", nombres)
	}
}

func TestEscribirCreaElDirectorio(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".cogo", "hondo", "x.json")
	if err := Escribir(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}
