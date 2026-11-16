package importar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

const docFixture = `# Guía del proyecto

Una intro que no afirma nada y se descarta.

## Decisión: usamos Postgres

Decidimos usar Postgres 16 porque el equipo ya lo opera y el pool de conexiones
se comporta bien con PgBouncer. Se eligió por sobre MySQL.

## Nunca se borra la tabla usuarios sin backup

Es obligatorio correr el backup diario antes de cualquier migración que toque
usuarios. Nunca se hace un drop directo.

### Cómo se despliega

Los pasos son: build de la imagen, push al registry, y force rebuild en el panel.
El deploy tarda unos tres minutos.

## Enlaces

- [Runbook](docs/runbook.md)
- [ADR 1](docs/adr/0001.md)

## Ojo: el cron duplica mails

Bug conocido: si el cron corre dos veces en el mismo minuto manda el mail dos
veces. El workaround es el lock en Redis.

## Corto

Poco.
`

func raizFixture(t *testing.T) string {
	t.Helper()
	raiz := t.TempDir()
	_ = os.MkdirAll(filepath.Join(raiz, "docs", "adr"), 0o755)
	_ = os.MkdirAll(filepath.Join(raiz, "node_modules", "x"), 0o755)
	escribir := func(rel, s string) {
		if err := os.WriteFile(filepath.Join(raiz, filepath.FromSlash(rel)), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribir("CLAUDE.md", docFixture)
	escribir("docs/adr/0001.md", "# ADR 1\n\n## Contexto y decisión\n\nAdoptamos Fastify porque mide 2x mejor que Express en nuestro benchmark del 2026-03-01.\n")
	escribir("node_modules/x/README.md", "## Esto no se importa\n\nPorque es de una dependencia y no del proyecto, aunque tenga cuarenta caracteres.\n")
	escribir("notas-sueltas.md", "## Tampoco\n\nUn .md fuera de docs/ y sin nombre conocido no se toma, aunque tenga cuerpo largo.\n")
	return raiz
}

func TestSeccionarPorH2yH3(t *testing.T) {
	secs := Seccionar("CLAUDE.md", []byte(docFixture))
	titulos := []string{}
	for _, s := range secs {
		titulos = append(titulos, s.Titulo)
	}
	quiero := []string{"Decisión: usamos Postgres", "Nunca se borra la tabla usuarios sin backup", "Cómo se despliega", "Enlaces", "Ojo: el cron duplica mails", "Corto"}
	if strings.Join(titulos, "|") != strings.Join(quiero, "|") {
		t.Fatalf("secciones: %v", titulos)
	}
	if secs[0].Linea != 5 || secs[2].Nivel != 3 {
		t.Fatalf("línea del primer H2 = %d (esperaba 5); nivel del H3 = %d", secs[0].Linea, secs[2].Nivel)
	}
	if strings.Contains(secs[0].Cuerpo, "Una intro") {
		t.Fatal("lo anterior al primer H2 no es de nadie")
	}
}

func TestTipoYAfirma(t *testing.T) {
	casos := []struct{ titulo, cuerpo, tipo string }{
		{"Decisión: usamos Postgres", "Decidimos usar Postgres 16 por el pool.", "decision"},
		{"Nunca se borra la tabla usuarios", "Es obligatorio el backup.", "constraint"},
		{"Cómo se despliega", "Los pasos son build, push y rebuild en el panel.", "runbook"},
		{"Ojo: el cron duplica mails", "Bug conocido con workaround en Redis.", "bug"},
		{"Arquitectura", "Tres servicios y una cola.", "architecture"},
		{"Cualquier otra cosa", "Prosa que describe cómo son las cosas sin palabras clave.", "architecture"},
		{"Build", "```bash\ngo build ./...\n```", "command"},
	}
	for _, c := range casos {
		if got := Tipo(c.titulo, c.cuerpo); got != c.tipo {
			t.Errorf("%q: esperaba %s, dio %s", c.titulo, c.tipo, got)
		}
	}
	if Afirma("Enlaces", "- [a](x)\n- [b](y)\n- [c](z)\n") {
		t.Error("una lista de enlaces no afirma nada")
	}
	if Afirma("Corto", "Poco.") {
		t.Error("dos palabras no son una nota")
	}
	if !Afirma("Ojo", "Bug conocido: si el cron corre dos veces manda el mail dos veces.") {
		t.Error("una frase con contenido sí afirma")
	}
}

// Todo lo importado nace amarillo, con origen instrumento y anclado al
// archivo y la línea de donde salió — y si alguien edita justo esa sección, la
// nota se entera.
func TestNotasNacenAmarillasYAncladas(t *testing.T) {
	raiz := raizFixture(t)
	hoy := core.NewDate(2026, 9, 23)
	notas, r, err := Notas(raiz, Opciones{Proyecto: "tienda", Hoy: hoy})
	if err != nil {
		t.Fatal(err)
	}
	// CLAUDE.md: 4 afirman (postgres, backup, despliega, cron); Enlaces y Corto no. ADR: 1.
	if len(notas) != 5 || r.Archivos != 2 || r.Saltadas["no afirma nada"] != 2 {
		t.Fatalf("notas=%d archivos=%d saltadas=%v creadas=%v", len(notas), r.Archivos, r.Saltadas, r.Creadas)
	}
	vault := map[string]*core.Note{}
	roots := core.SingleRoot(raiz)
	for _, n := range notas {
		if n.Origin != string(core.OrigenInstrumento) || n.Check.Test != "" || len(n.Evidence) != 1 || n.Evidence[0].Kind != "file_read" {
			t.Fatalf("nota mal armada: %+v", n)
		}
		core.StampEvidenceHashes(n, roots)
		if n.Evidence[0].Hash == "" || n.Evidence[0].Anchor == "" {
			t.Fatalf("%s: la cita tiene que quedar anclada: %+v", n.ID, n.Evidence[0])
		}
		vault[n.ID] = n
	}
	core.ResolveEvidence(vault, roots)
	for id, n := range vault {
		if v := core.Evaluate(n, vault, nil, hoy); v.Color != core.Yellow {
			t.Errorf("%s: todo lo importado nace amarillo, dio %s (%s)", id, v.Color, v.Reason)
		}
	}
	// Editar la sección del backup, y solo esa.
	p := filepath.Join(raiz, "CLAUDE.md")
	b, _ := os.ReadFile(p)
	nuevo := strings.Replace(string(b), "Nunca se hace un drop directo.", "Ahora sí se puede hacer un drop directo.", 1)
	if err := os.WriteFile(p, []byte(nuevo), 0o644); err != nil {
		t.Fatal(err)
	}
	core.ResolveEvidence(vault, roots)
	backup := core.DeriveID("tienda", "Nunca se borra la tabla usuarios sin backup")
	postgres := core.DeriveID("tienda", "Decisión: usamos Postgres")
	if vault[backup].Evidence[0].Status != core.EvDrifted {
		t.Fatalf("la nota cuya sección cambió tiene que derivar: %s", vault[backup].Evidence[0].Status)
	}
	if st := vault[postgres].Evidence[0].Status; st == core.EvDrifted {
		t.Fatalf("la nota de otra sección del mismo archivo no deriva: %s", st)
	}
	// Segunda vuelta: nada nuevo.
	existe := func(id string) bool { _, ok := vault[id]; return ok }
	otras, r2, _ := Notas(raiz, Opciones{Proyecto: "tienda", Hoy: hoy, Existe: existe})
	if len(otras) != 0 || r2.Saltadas["ya existía"] != 5 {
		t.Fatalf("importar dos veces no crea nada: %d %v", len(otras), r2.Saltadas)
	}
}
