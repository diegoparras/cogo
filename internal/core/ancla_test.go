package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El archivo de trabajo: una cita a la línea 5.
const archivoBase = `package tienda

import "fmt"

func Cobrar(monto int) error {
	if monto <= 0 {
		return fmt.Errorf("monto invalido")
	}
	return nil
}
`

func anclarEn(t *testing.T, contenido, ref string) (string, string) {
	t.Helper()
	a, at, ok := Anclar([]byte(contenido), ref)
	if !ok {
		t.Fatalf("no se pudo anclar %q", ref)
	}
	return a, at
}

// El caso que motiva todo: alguien toca una línea lejana y la nota se ponía
// amarilla. Con el ancla, el cambio se reconoce como ajeno a la cita.
func TestUnCambioLejosDeLaCitaNoEsMaterial(t *testing.T) {
	ref := "internal/tienda/cobro.go:5"
	ancla, at := anclarEn(t, archivoBase, ref)

	// Una línea de import pasa a ser un bloque de cuatro: todo lo de abajo se
	// corre tres líneas. La cita se movió, no cambió.
	nuevo := strings.Replace(archivoBase, `import "fmt"`, "import (\n\t\"fmt\"\n\t\"os\"\n)", 1)
	c := AnalizarCita([]byte(nuevo), ref, ancla, at)
	if c.Material {
		t.Fatalf("se consideró material un cambio que no toca la cita: %s", c.Motivo)
	}
	if c.Linea != 8 {
		t.Errorf("la cita se movió a la línea 8, se informó %d (%s)", c.Linea, c.Motivo)
	}
}

// Un cambio en otra parte del archivo, sin correr la cita de lugar.
func TestUnCambioQueNoCorreLaCitaTampocoEsMaterial(t *testing.T) {
	ref := "cobro.go:5"
	ancla, at := anclarEn(t, archivoBase, ref)

	nuevo := strings.Replace(archivoBase, "return nil", "return nil // ok", 1)
	c := AnalizarCita([]byte(nuevo), ref, ancla, at)
	if c.Material {
		t.Fatalf("un cambio en la línea 9 invalidó una cita a la 5: %s", c.Motivo)
	}
	if c.Linea != 0 {
		t.Errorf("la cita no se movió, pero se informó la línea %d", c.Linea)
	}
}

// Lo que SÍ tiene que avisar: cambió lo que la nota citaba.
func TestUnCambioEnLaCitaEsMaterial(t *testing.T) {
	ref := "cobro.go:5"
	ancla, at := anclarEn(t, archivoBase, ref)

	nuevo := strings.Replace(archivoBase, "func Cobrar(monto int) error {", "func Cobrar(monto, cuotas int) error {", 1)
	c := AnalizarCita([]byte(nuevo), ref, ancla, at)
	if !c.Material {
		t.Fatalf("cambió la firma citada y no se avisó: %s", c.Motivo)
	}
}

// Y si lo citado desapareció del archivo, también.
func TestSiLaCitaDesapareceEsMaterial(t *testing.T) {
	ref := "cobro.go:5"
	ancla, at := anclarEn(t, archivoBase, ref)

	nuevo := strings.Replace(archivoBase, "func Cobrar(monto int) error {", "func Facturar() error {", 1)
	if c := AnalizarCita([]byte(nuevo), ref, ancla, at); !c.Material {
		t.Fatalf("desapareció lo citado y no se avisó: %s", c.Motivo)
	}
}

// El formateador. Es la fuente número uno de amarillos que nadie mira.
func TestElEspaciadoNoEsUnCambio(t *testing.T) {
	ref := "cobro.go:6-8"
	ancla, at := anclarEn(t, archivoBase, ref)

	conEspacios := strings.ReplaceAll(archivoBase, "\t", "    ")      // tabs -> 4 espacios
	conEspacios = strings.ReplaceAll(conEspacios, " <= 0", "  <=  0") // alineación
	conEspacios = strings.ReplaceAll(conEspacios, "\n", "  \n")       // basura al final
	conEspacios = strings.ReplaceAll(conEspacios, "\n", "\r\n")       // y CRLF

	if c := AnalizarCita([]byte(conEspacios), ref, ancla, at); c.Material {
		t.Fatalf("un cambio de espaciado se tomó como material: %s", c.Motivo)
	}
}

// Pero la sangría no es solo espaciado en todos lados. En Python es sintaxis, y
// sacarla cambia qué hace el código.
func TestSacarLaSangriaSiEsUnCambio(t *testing.T) {
	py := "def cobrar(monto):\n    if monto <= 0:\n        raise ValueError('monto invalido')\n    return True\n"
	ref := "cobro.py:3"
	ancla, at := anclarEn(t, py, ref)

	sinSangria := strings.Replace(py, "        raise ValueError", "raise ValueError", 1)
	if c := AnalizarCita([]byte(sinSangria), ref, ancla, at); !c.Material {
		t.Fatalf("se sacó la sangría de una línea de Python y se tomó como cosmético: %s", c.Motivo)
	}
}

// La trampa del método: una cita de una línea que dice "}" coincide en cualquier
// parte del archivo. Si COGO la relocalizara, estaría adivinando — y absolvería
// un cambio real. Prefiere avisar.
func TestUnaCitaPocoDistintivaNoSeRelocaliza(t *testing.T) {
	ref := "cobro.go:8" // la línea 8 es "}"
	ancla, at := anclarEn(t, archivoBase, ref)

	// Se agrega una línea arriba de todo: el "}" se corre a la 9, y en la 8 queda
	// otra cosa. Buscar "}" encontraría dos, y aun con uno solo sería adivinar.
	nuevo := "// (c) tienda\n" + archivoBase
	c := AnalizarCita([]byte(nuevo), ref, ancla, at)
	if !c.Material {
		t.Fatalf("se relocalizó una cita que coincide en cualquier lado: %s", c.Motivo)
	}
}

// Lo mismo cuando el texto citado aparece dos veces: de un empate no se concluye
// a cuál de los dos se movió.
func TestUnaCitaRepetidaNoSeRelocaliza(t *testing.T) {
	original := "alfa uno dos tres\nbravo cuatro cinco\ncharlie seis siete\n"
	ref := "x.txt:2"
	ancla, at := anclarEn(t, original, ref)

	// La línea 2 pasó a decir otra cosa, y el texto citado quedó dos veces.
	duplicado := "zulu\nyankee ocho nueve\nbravo cuatro cinco\ndelta\nbravo cuatro cinco\n"
	if c := AnalizarCita([]byte(duplicado), ref, ancla, at); !c.Material {
		t.Fatalf("se eligió una de dos apariciones idénticas: %s", c.Motivo)
	}
}

// Sin ancla no se puede juzgar, y no poder juzgar no es lo mismo que absolver.
func TestSinAnclaSeAvisaIgual(t *testing.T) {
	if c := AnalizarCita([]byte(archivoBase), "cobro.go:5", "", ""); !c.Material {
		t.Fatalf("sin ancla se absolvió el cambio: %s", c.Motivo)
	}
}

// Si alguien edita la cita sin re-verificar, el ancla habla de otra región:
// compararlas sería mezclar dos preguntas.
func TestEditarLaCitaObligaAReverificar(t *testing.T) {
	ancla, at := anclarEn(t, archivoBase, "cobro.go:5")
	if c := AnalizarCita([]byte(archivoBase), "cobro.go:9", ancla, at); !c.Material {
		t.Fatalf("se cambió la cita y se siguió comparando contra el ancla vieja: %s", c.Motivo)
	}
}

// Una cita sin número de línea cita el archivo entero: ahí sí, cualquier cambio
// de contenido es material — pero el espaciado sigue sin serlo.
func TestUnaCitaSinLineaCitaTodoElArchivo(t *testing.T) {
	ref := "cobro.go"
	ancla, at := anclarEn(t, archivoBase, ref)

	if c := AnalizarCita([]byte(strings.ReplaceAll(archivoBase, "\t", "  ")), ref, ancla, at); c.Material {
		t.Errorf("reindentar el archivo entero se tomó como material: %s", c.Motivo)
	}
	nuevo := archivoBase + "\nfunc Otra() {}\n"
	if c := AnalizarCita([]byte(nuevo), ref, ancla, at); !c.Material {
		t.Errorf("se agregó una función y no se avisó: %s", c.Motivo)
	}
}

func TestLocalizadorDeLasFormasQueSeUsan(t *testing.T) {
	casos := map[string]string{
		"a/b/c.go:42":                    "42",
		"a/b/c.go:120-140":               "120-140",
		"c.go#L42":                       "42",
		"docker-compose.yml line 33":     "33",
		"docker-compose.yml lines 33-41": "33-41",
		"docker-compose.yml:164 — REDIS_URL: redis://x": "164",
		"notas/algo.md":      "",
		"conecta OK a redis": "",
	}
	for ref, quiero := range casos {
		if got := localizadorDe(ref); got != quiero {
			t.Errorf("localizadorDe(%q) = %q, se esperaba %q", ref, got, quiero)
		}
	}
}

// La prueba de punta a punta sobre el pipeline real: una nota verde cuyo archivo
// citado cambia lejos de la cita tiene que SEGUIR verde, y ponerse amarilla
// cuando el cambio la alcanza.
func TestElColorSoloBajaCuandoElCambioAlcanzaLaCita(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "cobro.go")
	if err := os.WriteFile(ruta, []byte(archivoBase), 0o644); err != nil {
		t.Fatal(err)
	}
	hoy := MustDate("2026-08-03")
	roots := SingleRoot(dir)

	nota := func() *Note {
		return &Note{
			ID: "cobro-valida-monto", Type: "architecture", Project: "tienda", LastVerified: hoy,
			Evidence: []Evidence{{Kind: "file_read", Ref: "cobro.go:5"}},
			Check:    Check{Test: "go test ./tienda", Status: "passed"},
			Body:     "## Claim\nCobrar valida el monto.",
		}
	}
	n := nota()
	StampEvidenceHashes(n, roots)
	if n.Evidence[0].Anchor == "" {
		t.Fatal("verificar no dejó ancla")
	}

	// Un cambio lejano que además corre la cita de lugar.
	nuevo := strings.Replace(archivoBase, `import "fmt"`, "import (\n\t\"fmt\"\n\t\"os\"\n)", 1)
	if err := os.WriteFile(ruta, []byte(nuevo), 0o644); err != nil {
		t.Fatal(err)
	}
	vault := map[string]*Note{n.ID: n}
	ResolveEvidence(vault, roots)
	if got := EvaluateVault(vault, nil, hoy)[n.ID]; got.Color != Green {
		t.Errorf("un cambio ajeno a la cita bajó la nota a %s: %s — %s", got.Color, got.Reason, n.Evidence[0].Detail)
	}
	if len(MovedRefs(n)) != 1 {
		t.Errorf("la cita se movió y no se informó dónde quedó")
	}

	// Y ahora el cambio que sí la toca, desde el archivo original otra vez.
	if err := os.WriteFile(ruta, []byte(archivoBase), 0o644); err != nil {
		t.Fatal(err)
	}
	m := nota()
	StampEvidenceHashes(m, roots)
	roto := strings.Replace(archivoBase, "func Cobrar(monto int) error {", "func Cobrar(monto, cuotas int) error {", 1)
	if err := os.WriteFile(ruta, []byte(roto), 0o644); err != nil {
		t.Fatal(err)
	}
	vault = map[string]*Note{m.ID: m}
	ResolveEvidence(vault, roots)
	if got := EvaluateVault(vault, nil, hoy)[m.ID]; got.Color != Yellow {
		t.Errorf("cambió la línea citada y la nota quedó en %s: %s", got.Color, got.Reason)
	}
}

// El backfill: una nota escrita antes de que existieran las anclas se ancla sola
// la primera vez que se la resuelve con el archivo intacto — sin re-verificar
// nada y sin moverle el color a nadie. Si no fuera así, la materialidad solo
// serviría para las notas nuevas.
func TestLasNotasViejasSeAnclanSolas(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "cobro.go")
	if err := os.WriteFile(ruta, []byte(archivoBase), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := SingleRoot(dir)
	hoy := MustDate("2026-08-03")

	// Así se veía una nota verificada por la versión anterior: hash del archivo,
	// sin ancla.
	n := &Note{
		ID: "vieja", Type: "architecture", Project: "tienda", LastVerified: hoy,
		Evidence: []Evidence{{Kind: "file_read", Ref: "cobro.go:5", Hash: fileHash(ruta)}},
		Check:    Check{Test: "t", Status: "passed"}, Body: "## Claim\nx",
	}
	vault := map[string]*Note{n.ID: n}
	ResolveEvidence(vault, roots)
	if n.Evidence[0].Anchor == "" {
		t.Fatal("una nota vieja con el archivo intacto no se ancló sola")
	}
	if got := EvaluateVault(vault, nil, hoy)[n.ID]; got.Color != Green {
		t.Fatalf("anclar solo le cambió el color: %s", got.Color)
	}

	// Y desde ahí ya tiene materialidad.
	nuevo := strings.Replace(archivoBase, "return nil", "return nil // ok", 1)
	if err := os.WriteFile(ruta, []byte(nuevo), 0o644); err != nil {
		t.Fatal(err)
	}
	ResolveEvidence(vault, roots)
	if n.Evidence[0].Status != EvMoved {
		t.Errorf("la nota anclada al vuelo no aprovechó el ancla: status=%s (%s)",
			n.Evidence[0].Status, n.Evidence[0].Detail)
	}
}

// El ancla tiene que sobrevivir al viaje por el frontmatter, o el backfill se
// haría de nuevo en cada arranque.
func TestElAnclaSobreviveElArchivo(t *testing.T) {
	n := &Note{
		ID: "x", Type: "architecture", LastVerified: MustDate("2026-08-03"),
		Evidence: []Evidence{{Kind: "file_read", Ref: "c.go:5", Hash: "abc", Anchor: "def", AnchorAt: "5"}},
		Body:     "## Claim\nx",
	}
	b, err := MarshalNote(n)
	if err != nil {
		t.Fatal(err)
	}
	leida, err := ParseNote(b)
	if err != nil {
		t.Fatal(err)
	}
	got := leida.Evidence[0]
	if got.Anchor != "def" || got.AnchorAt != "5" {
		t.Errorf("el ancla no sobrevivió: anchor=%q anchor_at=%q\n%s", got.Anchor, got.AnchorAt, b)
	}
}
