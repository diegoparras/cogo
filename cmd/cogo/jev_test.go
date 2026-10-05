package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/jev"
)

// juezDePrueba contesta lo programado y se abstiene de todo lo demás.
type juezDePrueba struct {
	pertinencia map[string]float64 // por id de nota (claim contiene el id)
}

func (juezDePrueba) Disponible() bool { return true }
func (j juezDePrueba) Pertinencia(_ context.Context, claim, _ string) (float64, bool) {
	for k, p := range j.pertinencia {
		if strings.Contains(claim, k) {
			return p, true
		}
	}
	return 0, false
}
func (juezDePrueba) Clase(context.Context, string) (string, bool)                { return "", false }
func (juezDePrueba) NivelDeEvidenciaEnCache(string) (string, bool)               { return "", false }
func (juezDePrueba) ChequeoEnCache(string, string) (float64, bool)               { return 0, false }
func (juezDePrueba) Contradicen(context.Context, string, string) (float64, bool) { return 0, false }
func (juezDePrueba) Tacticas(context.Context, string, []jev.Tecnica) (map[string]float64, error) {
	return nil, nil
}
func (juezDePrueba) Radiografia(context.Context, string) (string, string, bool) { return "", "", false }
func (juezDePrueba) Seccion(context.Context, string, string) (string, bool, bool) {
	return "", false, false
}
func (juezDePrueba) Precalentar(context.Context, []jev.NotaParaJuzgar) (int, error) {
	return 0, nil
}

// Una nota verde que no habla de la acción no la respalda. Sin Jev, authorize
// miraba el color y decía que sí; con Jev, la saca y lo dice.
func TestAuthorizeDescartaRespaldoNoPertinente(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro) // el motor y el registro quedan globales: no dejárselos al test siguiente
	dir := t.TempDir()
	notaVerificada(t, dir, "redis-vive", "REDIS: Redis vive en fisherboy-redis:6379 y responde PING.")
	notaVerificada(t, dir, "usuarios-respaldada", "USUARIOS: la tabla usuarios tiene backup diario verificado; borrarla y restaurar es seguro.")
	conVault(&dir)
	if err := instalarMotor(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { juezDelProceso = nil; engancharJuez() })
	juezDelProceso = juezDePrueba{pertinencia: map[string]float64{"REDIS": 0.05, "USUARIOS": 0.95}}
	pars.Poner("jev.activo", true, "test")
	t.Cleanup(func() { pars.Restaurar("jev.activo", "test") })

	d := nuevasDeps(dir)
	// Cita las dos: la de Redis no cuenta, la de usuarios sí → autoriza.
	v, texto, err := decidirAutorizacion(context.Background(), d, authorizeIn{
		Action: "drop table usuarios", Notes: []string{"redis-vive", "usuarios-respaldada"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Autoriza || !strings.Contains(texto, "not counted as support (Jev: they do not bear on this action): redis-vive") {
		t.Fatalf("esperaba autorizado con redis descartada:\n%s", texto)
	}
	// Solo la de Redis: sin respaldo pertinente → no autoriza.
	v, texto, err = decidirAutorizacion(context.Background(), d, authorizeIn{
		Action: "drop table usuarios", Notes: []string{"redis-vive"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Autoriza || !strings.HasPrefix(texto, "NOT AUTHORIZED") {
		t.Fatalf("una nota verde de otra cosa no respalda:\n%s", texto)
	}
	_ = filepath.Join
}
