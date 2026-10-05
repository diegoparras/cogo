package accion

import "testing"

// El voto externo entra con la misma regla que los otros dos: manda la más
// estricta. Puede endurecer; nunca aflojar. Y con el texto mudo y nada
// declarado, no resuelve la duda hacia abajo.
func TestElVotoExternoSoloEndurece(t *testing.T) {
	t.Cleanup(func() { SetClasificadorExterno(nil) })
	votar := func(c Clase) { SetClasificadorExterno(func(string) (Clase, bool) { return c, true }) }

	// Texto mudo + clase declarada baja: es el caso que el regex no ve.
	votar(Irreversible)
	if c, _ := Decidir("informative", "vaciar el bucket de producción"); c != Irreversible {
		t.Fatalf("el juez tiene que poder subir una declaración baja: %s", c)
	}
	// Texto mudo + nada declarado: sigue pidiendo el máximo, aunque el juez
	// diga informativa. Un juez equivocado ahí sería un borrado autorizado.
	votar(Informativa)
	if c, _ := Decidir("", "hacer una cosa"); c != Irreversible {
		t.Fatalf("lo desconocido sigue pidiendo el máximo: %s", c)
	}
	// Regex dice irreversible, juez dice reversible: manda el regex.
	votar(Reversible)
	if c, _ := Decidir("", "rm -rf build"); c != Irreversible {
		t.Fatalf("el juez no puede aflojar lo que el texto delata: %s", c)
	}
	// Regex dice reversible, juez dice costosa: manda el juez.
	votar(Costosa)
	if c, por := Decidir("", "editar el archivo de la migración"); c != Costosa || por == "" {
		t.Fatalf("el juez endurece: %s (%s)", c, por)
	}
	// Se abstiene: todo como antes.
	SetClasificadorExterno(func(string) (Clase, bool) { return "", false })
	if c, _ := Decidir("informative", "hacer una cosa"); c != Informativa {
		t.Fatalf("con abstención vale lo declarado: %s", c)
	}
}
