package veredicto

import "testing"

func TestPonerYBalancear(t *testing.T) {
	st := Abrir(t.TempDir())
	if err := st.Poner(Veredicto{Recibo: "r-1", Acertado: true, Quien: "diego"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Poner(Veredicto{Recibo: "r-2", Acertado: false, Quien: "diego"}); err != nil {
		t.Fatal(err)
	}
	// Cambiar de opinión: gana el último.
	if err := st.Poner(Veredicto{Recibo: "r-2", Acertado: true, Quien: "diego"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Poner(Veredicto{Recibo: "", Acertado: true}); err == nil {
		t.Fatal("sin recibo no hay veredicto")
	}
	vs, err := st.Todos()
	if err != nil || len(vs) != 2 || !vs["r-2"].Acertado || vs["r-1"].Cuando.IsZero() {
		t.Fatalf("todos: %v %v", vs, err)
	}
	// r-1 bloqueó y estuvo bien; r-2 dejó pasar y estuvo bien; r-3 bloqueó y
	// nadie opinó; r-4 dejó pasar y el humano dice que no debía.
	_ = st.Poner(Veredicto{Recibo: "r-4", Acertado: false, Quien: "diego"})
	vs, _ = st.Todos()
	b := Balancear(map[string]bool{"r-1": false, "r-2": true, "r-3": false, "r-4": true}, vs)
	if b.TotalDecidido != 4 || b.Juzgados != 3 || b.SinJuzgar != 1 || b.BloqueosBien != 1 || b.PermisosBien != 1 || b.PermisosMal != 1 || b.BloqueosMal != 0 {
		t.Fatalf("balance: %+v", b)
	}
}
