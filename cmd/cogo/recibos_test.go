package main

import (
	"context"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/recibo"
)

// Cada authorize deja su recibo, atado a la cabeza del registro, y el agente
// recibe el id para citarlo.
func TestAuthorizeDejaRecibo(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	dir := t.TempDir()
	notaVerificada(t, dir, "build-se-regenera", "La carpeta build se regenera con go build: borrarla es seguro.")
	conVault(&dir)
	if err := instalarMotor(dir); err != nil {
		t.Fatal(err)
	}
	d := nuevasDeps(dir)
	v, texto, err := decidirAutorizacion(context.Background(), d, authorizeIn{
		Action: "rm -rf build", Notes: []string{"build-se-regenera"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Autoriza || !strings.Contains(texto, "receipt: r-") {
		t.Fatalf("esperaba autorizado con recibo:\n%s", texto)
	}
	todos, err := recibo.Abrir(dir).Todos()
	if err != nil || len(todos) != 1 {
		t.Fatalf("un recibo: %v %d", err, len(todos))
	}
	r := todos[0]
	if r.Accion != "rm -rf build" || r.Clase != "irreversible" || !r.Autoriza || len(r.Notas) != 1 ||
		r.Notas[0].ID != "build-se-regenera" || r.Notas[0].Final != "verified" || r.Notas[0].Eventos != "verified" || r.Seq == 0 || r.Cabeza == "" {
		t.Fatalf("recibo mal armado: %+v", r)
	}
	j, _ := journalDe(dir)
	if rec := recibo.Reconstruir(j, r); !rec.Fiel {
		t.Fatalf("recién escrito tiene que ser fiel: %+v", rec)
	}
}
