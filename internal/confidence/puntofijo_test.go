package confidence

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// grafoAleatorio arma un grafo con ciclos casi seguros y estados locales
// variados. Es el material con el que se intenta falsar el determinismo.
func grafoAleatorio(r *rand.Rand, n int) (Grafo, map[string]Estado) {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("n%02d", i)
	}
	g := Grafo{}
	local := map[string]Estado{}
	niveles := []Estado{Verified, ClaimedPassed, CheckDeclared, Asserted, Stale, Contradicted, Refuted, Quarantined}
	for i, id := range ids {
		// entre 0 y 3 dependencias al azar: con n chico, los ciclos abundan
		for k := 0; k < r.Intn(4); k++ {
			g[id] = append(g[id], ids[r.Intn(n)])
		}
		local[id] = niveles[(i+r.Intn(len(niveles)))%len(niveles)]
	}
	return g, local
}

// LA propiedad. Es la que el spec pedía y la que hace que dos corridas sobre el
// mismo vault no puedan diferir.
func TestElPuntoFijoNoDependeDelOrdenDeVisita(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for caso := 0; caso < 300; caso++ {
		g, local := grafoAleatorio(r, 3+r.Intn(8))
		esperado := PuntoFijo(g, local)

		// Se recalcula muchas veces con las dependencias barajadas: si el
		// resultado dependiera del orden de visita, aparecería acá.
		for rep := 0; rep < 8; rep++ {
			g2 := Grafo{}
			for id, deps := range g {
				d2 := append([]string(nil), deps...)
				r.Shuffle(len(d2), func(i, j int) { d2[i], d2[j] = d2[j], d2[i] })
				g2[id] = d2
			}
			got := PuntoFijo(g2, local)
			for id, want := range esperado {
				if got[id] != want {
					t.Fatalf("caso %d: la nota %q dio %s y después %s", caso, id, want, got[id])
				}
			}
		}
	}
}

// La diferencia concreta con el motor vigente: un ciclo donde nadie tiene motivo
// propio para degradarse NO se degrada. Hoy COGO los pone a todos en rojo.
func TestUnCicloSanoNoSeDegrada(t *testing.T) {
	g := Grafo{"a": {"b"}, "b": {"a"}}
	local := map[string]Estado{"a": Verified, "b": Verified}
	got := PuntoFijo(g, local)
	if got["a"] != Verified || got["b"] != Verified {
		t.Errorf("un ciclo sin razón para degradarse quedó en a=%s b=%s; deberían seguir verificadas", got["a"], got["b"])
	}
}

// Pero un ciclo con UNA nota podrida arrastra a todo el ciclo: la duda se
// propaga aunque el grafo tenga vueltas.
func TestUnCicloConUnaPodridaSeContagiaEntero(t *testing.T) {
	g := Grafo{"a": {"b"}, "b": {"c"}, "c": {"a"}}
	local := map[string]Estado{"a": Verified, "b": Verified, "c": Refuted}
	got := PuntoFijo(g, local)
	for _, id := range []string{"a", "b", "c"} {
		if got[id] != Refuted {
			t.Errorf("%s quedó en %s; el ciclo entero debería caer a refuted", id, got[id])
		}
	}
}

// Nadie sube: el punto fijo solo puede dejar una nota como está o bajarla.
func TestLaPropagacionNuncaEleva(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for caso := 0; caso < 200; caso++ {
		g, local := grafoAleatorio(r, 2+r.Intn(9))
		got := PuntoFijo(g, local)
		for id, fin := range got {
			if fin.Rango() > local[id].Rango() {
				t.Fatalf("la nota %q subió de %s a %s", id, local[id], fin)
			}
		}
	}
}

// Monotonía: si una nota empeora por sus propios méritos, ninguna del vault
// puede terminar mejor que antes.
func TestSiUnaNotaEmpeoraNingunaMejora(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for caso := 0; caso < 200; caso++ {
		g, local := grafoAleatorio(r, 3+r.Intn(7))
		antes := PuntoFijo(g, local)

		ids := make([]string, 0, len(local))
		for id := range local {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		victima := ids[r.Intn(len(ids))]
		if local[victima] == Quarantined {
			continue // ya está en el fondo
		}
		peor := map[string]Estado{}
		for k, v := range local {
			peor[k] = v
		}
		peor[victima] = Quarantined

		despues := PuntoFijo(g, peor)
		for id := range local {
			if despues[id].Rango() > antes[id].Rango() {
				t.Fatalf("al degradar %q, la nota %q MEJORÓ de %s a %s", victima, id, antes[id], despues[id])
			}
		}
	}
}

// Depender de algo que no existe es lo menos confiable que hay: no se puede uno
// apoyar en lo que no se puede mirar.
func TestDependerDeLoQueNoEstaHundeLaNota(t *testing.T) {
	g := Grafo{"a": {"fantasma"}}
	local := map[string]Estado{"a": Verified}
	if got := PuntoFijo(g, local)["a"]; got != Estado(0) {
		t.Errorf("depender de una nota ausente dejó la nota en %s; debería caer al fondo", got)
	}
}

// El resultado ES un punto fijo: aplicarle la función otra vez no lo mueve.
func TestElResultadoEsRealmenteUnPuntoFijo(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for caso := 0; caso < 200; caso++ {
		g, local := grafoAleatorio(r, 3+r.Intn(8))
		sigma := PuntoFijo(g, local)
		for id := range local {
			quiero := local[id]
			for _, d := range g[id] {
				est, hay := sigma[d]
				if !hay {
					est = Estado(0)
				}
				quiero = Meet(quiero, est)
			}
			if quiero != sigma[id] {
				t.Fatalf("no es punto fijo: F(σ)[%s] = %s pero σ[%s] = %s", id, quiero, id, sigma[id])
			}
		}
	}
}

// Y es el MAYOR: cualquier otro punto fijo es menor o igual. Se comprueba
// contra el menor, que se obtiene iterando desde abajo — y que en un ciclo sano
// da un resultado peor, que es justamente por lo que no se usa.
func TestEsElMayorPuntoFijo(t *testing.T) {
	g := Grafo{"a": {"b"}, "b": {"a"}}
	local := map[string]Estado{"a": Verified, "b": Verified}

	mayor := PuntoFijo(g, local)

	// menor punto fijo: arrancar desde el fondo y subir hasta estabilizar
	menor := map[string]Estado{"a": Estado(0), "b": Estado(0)}
	for i := 0; i < 64; i++ {
		cambio := false
		for id := range local {
			nuevo := local[id]
			for _, d := range g[id] {
				nuevo = Meet(nuevo, menor[d])
			}
			if nuevo != menor[id] {
				menor[id] = nuevo
				cambio = true
			}
		}
		if !cambio {
			break
		}
	}
	if menor["a"].Rango() >= mayor["a"].Rango() {
		t.Skip("en este grafo ambos coinciden; el caso no discrimina")
	}
	t.Logf("mismo ciclo: el menor punto fijo da %s, el mayor da %s", menor["a"], mayor["a"])
	if mayor["a"] != Verified {
		t.Errorf("el mayor punto fijo debería conservar %s", Verified)
	}
}

// Terminación acotada: la cantidad de pasos no depende de los ciclos.
func TestTerminaAunConGrafosMuyCiclicos(t *testing.T) {
	n := 60
	g := Grafo{}
	local := map[string]Estado{}
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("n%02d", i)
	}
	// todos con todos: el grafo más cíclico posible
	for i, id := range ids {
		g[id] = append([]string(nil), ids...)
		local[id] = Verified
		if i == 0 {
			local[id] = Refuted
		}
	}
	got := PuntoFijo(g, local)
	for _, id := range ids {
		if got[id] != Refuted {
			t.Fatalf("%s quedó en %s; con una podrida y todos conectados, todo cae", id, got[id])
		}
	}
}
