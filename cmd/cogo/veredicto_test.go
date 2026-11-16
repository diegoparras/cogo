package main

import (
	"context"
	"testing"

	"github.com/diegoparras/cogo/internal/recibo"
	"github.com/diegoparras/cogo/internal/veredicto"
)

// El humano dice si COGO tenía razón sobre un recibo; eso es lo único que
// convierte un bloqueo en "lo que COGO evitó".
func TestVeredictoSobreUnRecibo(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	dir := t.TempDir()
	notaVerificada(t, dir, "build-se-regenera", "La carpeta build se regenera con go build: borrarla es seguro.")
	conVault(&dir)
	if err := instalarMotor(dir); err != nil {
		t.Fatal(err)
	}
	d := nuevasDeps(dir)
	// Una decisión sin respaldo: bloqueada.
	if v, _, err := decidirAutorizacion(context.Background(), d, authorizeIn{Action: "drop table usuarios"}); err != nil || v.Autoriza {
		t.Fatalf("esperaba bloqueo: %+v %v", v, err)
	}
	todos, _ := recibo.Abrir(dir).Todos()
	if len(todos) != 1 {
		t.Fatalf("un recibo: %d", len(todos))
	}
	id := todos[0].ID
	if err := cmdVeredicto([]string{"-vault", dir, id, "bien", "-nota", "era producción"}); err != nil {
		t.Fatal(err)
	}
	vs, _ := veredicto.Abrir(dir).Todos()
	if v, ok := vs[id]; !ok || !v.Acertado || v.Nota != "era producción" || v.Quien == "" {
		t.Fatalf("veredicto: %+v", vs)
	}
	if err := cmdVeredicto([]string{"-vault", dir, "r-no-existe", "bien"}); err == nil {
		t.Fatal("un recibo inexistente no se juzga")
	}
	if err := cmdVeredicto([]string{"-vault", dir, id, "quizás"}); err == nil {
		t.Fatal("bien o mal, no otra cosa")
	}
	if err := balanceDeVeredictos(dir); err != nil {
		t.Fatal(err)
	}
}
