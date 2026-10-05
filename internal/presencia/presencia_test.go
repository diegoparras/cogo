package presencia

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/lease"
)

// vaultCon escribe una auditoría como la que deja el servidor.
func vaultCon(t *testing.T, lineas ...map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, l := range lineas {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(dir, ".cogo", "audit.jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func hace(d time.Duration) string {
	return time.Now().UTC().Add(-d).Format(time.RFC3339)
}

func TestVeQuienEstaYQueEstaHaciendo(t *testing.T) {
	dir := vaultCon(t,
		map[string]string{"time": hace(2 * time.Minute), "caller": "maquina-A", "tool": "pack", "proyecto": "cogo"},
		map[string]string{"time": hace(1 * time.Minute), "caller": "maquina-B", "tool": "capture", "nota": "db-migracion", "proyecto": "cogo"},
	)
	ags := Ver(dir, time.Now().Add(-30*time.Minute))
	if len(ags) != 2 {
		t.Fatalf("esperaba 2 agentes, hay %d: %+v", len(ags), ags)
	}
	// Ordenados por actividad más reciente: B llamó después que A.
	if ags[0].Token != "maquina-B" {
		t.Errorf("el más reciente debería ir primero, vino %q", ags[0].Token)
	}
	if !ags[0].Escribiendo() {
		t.Error("capture es una escritura y debería contar como tal")
	}
	if ags[1].Escribiendo() {
		t.Error("pack no escribe nada; marcarlo como escritor haría ruidoso el aviso")
	}
	if len(ags[0].Notas) != 1 || ags[0].Notas[0] != "db-migracion" {
		t.Errorf("perdió sobre qué llamó: %+v", ags[0].Notas)
	}
}

// La ventana es lo que separa "está pasando" de "pasó". Sin esto el aviso
// hablaría de gente que ya terminó, y un aviso que aparece siempre no se lee.
func TestFueraDeLaVentanaNoCuenta(t *testing.T) {
	dir := vaultCon(t,
		map[string]string{"time": hace(3 * time.Hour), "caller": "maquina-vieja", "tool": "capture"},
	)
	if ags := Ver(dir, time.Now().Add(-30*time.Minute)); len(ags) != 0 {
		t.Fatalf("esperaba a nadie, vinieron %+v", ags)
	}
}

// Un llamado sin token no se le puede atribuir a nadie: presentarlo como agente
// sería inventar una sesión.
func TestAnonimoNoEsUnAgente(t *testing.T) {
	dir := vaultCon(t,
		map[string]string{"time": hace(time.Minute), "caller": "anon", "tool": "pack"},
		map[string]string{"time": hace(time.Minute), "caller": "", "tool": "pack"},
	)
	if ags := Ver(dir, time.Now().Add(-30*time.Minute)); len(ags) != 0 {
		t.Fatalf("esperaba a nadie, vinieron %+v", ags)
	}
}

func TestSinAuditoriaNoRompe(t *testing.T) {
	if ags := Ver(t.TempDir(), time.Now().Add(-time.Hour)); len(ags) != 0 {
		t.Fatalf("un vault sin auditoría no tiene agentes, vinieron %+v", ags)
	}
}

func TestOtrosMeSacaAMi(t *testing.T) {
	ags := []Agente{{Token: "yo"}, {Token: "el-otro"}}
	otros := Otros(ags, "yo")
	if len(otros) != 1 || otros[0].Token != "el-otro" {
		t.Fatalf("avisarle a alguien de sí mismo es ruido: %+v", otros)
	}
}

func TestEnProyectoFiltraPeroDejaAlDesconocido(t *testing.T) {
	ags := []Agente{
		{Token: "mismo", Proyectos: []string{"cogo"}},
		{Token: "otro-repo", Proyectos: []string{"talento"}},
		{Token: "solo-leyo"}, // no se sabe dónde está
	}
	got := EnProyecto(ags, "cogo")
	if len(got) != 2 || got[0].Token != "mismo" || got[1].Token != "solo-leyo" {
		t.Fatalf("ante la duda hay que avisar; ante la certeza de que está en otro lado, no: %+v", got)
	}
	// Sin proyecto declarado no se puede filtrar: pasan todos.
	if len(EnProyecto(ags, "")) != 3 {
		t.Error("sin proyecto no hay con qué filtrar")
	}
}

// ── el aviso ────────────────────────────────────────────────────────────────

// La regla que lo hace útil: si no hay nada que decir, no se dice nada.
func TestSinNadieNoHayAviso(t *testing.T) {
	if s := Aviso(nil, nil, "yo", time.Now()); s != "" {
		t.Fatalf("un aviso que aparece siempre es un aviso que nadie lee; vino %q", s)
	}
	// Un permiso PROPIO tampoco es noticia: para eso se tomó.
	mio := []lease.Lease{{Name: "migrar", Holder: "yo", Expires: "2099-01-01T00:00:00Z"}}
	if s := Aviso(nil, mio, "yo", time.Now()); s != "" {
		t.Fatalf("el permiso propio no se avisa; vino %q", s)
	}
}

func TestAvisoNombraAlOtroYSuPermiso(t *testing.T) {
	otros := []Agente{{Token: "maquina-B", Ultima: time.Now().Add(-2 * time.Minute),
		Escrituras: 3, Notas: []string{"db-migracion"}, Herramientas: []string{"capture"}}}
	ajeno := []lease.Lease{{Name: "migrar-db", Holder: "maquina-B",
		Expires: "2099-01-01T00:00:00Z", Note: "moviendo la tabla usuarios"}}

	s := Aviso(otros, ajeno, "yo", time.Now())
	for _, esperado := range []string{"maquina-B", "migrar-db", "db-migracion", "WRITING", "moviendo la tabla usuarios"} {
		if !strings.Contains(s, esperado) {
			t.Errorf("el aviso no dice %q:\n%s", esperado, s)
		}
	}
}

// ── el choque ───────────────────────────────────────────────────────────────

func TestChoqueSoloPorNombreExacto(t *testing.T) {
	ls := []lease.Lease{{Name: "migrar-db", Holder: "maquina-B", Expires: "2099-01-01T00:00:00Z"}}

	if _, hay := Choque("run the migrar-db script on staging", ls, "yo"); !hay {
		t.Error("la acción nombra el permiso: eso no es una sospecha, es la misma cosa")
	}
	if _, hay := Choque("MIGRAR-DB ahora", ls, "yo"); !hay {
		t.Error("la comparación tiene que ser insensible a mayúsculas")
	}
	if _, hay := Choque("deploy the frontend", ls, "yo"); hay {
		t.Error("un bloqueo falso en una herramienta de seguridad se paga con que la apaguen")
	}
	if _, hay := Choque("run the migrar-db script", ls, "maquina-B"); hay {
		t.Error("el propio permiso no choca: para eso se tomó")
	}
}

// Un permiso sin nombre no puede chocar con nada: si chocara, el substring
// vacío haría que TODA acción quedara bloqueada.
func TestPermisoSinNombreNoBloqueaTodo(t *testing.T) {
	ls := []lease.Lease{{Name: "   ", Holder: "otro", Expires: "2099-01-01T00:00:00Z"}}
	if _, hay := Choque("cualquier cosa", ls, "yo"); hay {
		t.Fatal("un permiso sin nombre bloquearía todas las acciones del vault")
	}
}
