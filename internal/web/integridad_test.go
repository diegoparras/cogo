package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

// Los tres agujeros de integridad que encontró la Fase 0. Cada uno de estos
// tests falla contra el código anterior a este cambio: son de regresión, no de
// cobertura.

// 1. Verificar es una DECLARACIÓN y queda asentada como tal, con su autor.
// Antes, un verde declarado y uno ejecutado eran indistinguibles.
func TestVerifyQuedaComoDeclaracion(t *testing.T) {
	s := testServer(t)
	rec := call(s.handleVerify, "POST", "/api/verify?id=redis", nil)
	if rec.Code != 200 {
		t.Fatalf("verify devolvió %d: %s", rec.Code, rec.Body.String())
	}
	n, err := core.ReadNoteFile(filepath.Join(s.dir, "redis.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := n.Check.Attestation(); got != core.AttestDeclared {
		t.Errorf("procedencia = %q, se esperaba %q", got, core.AttestDeclared)
	}
	if n.Check.AttestedBy == "" {
		t.Error("no quedó registrado quién lo declaró")
	}
	if n.Check.Attested == core.AttestExecuted {
		t.Error("GRAVE: verify se atribuyó una ejecución que no ocurrió")
	}
}

// 2. Una nota cuya evidencia derivó no se puede re-verificar sin confirmar
// contra qué. Antes, verificar borraba el aviso de deriva en silencio.
func TestVerifyNoBorraLaDeriva(t *testing.T) {
	s := testServer(t)
	// La deriva no se simula escribiendo Status: ese campo se recalcula al
	// cargar. Hace falta un archivo real que cambie.
	archivo := filepath.Join(s.dir, "config.yml")
	if err := os.WriteFile(archivo, []byte("host: viejo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Ruta absoluta: una relativa necesitaría una raíz de evidencia configurada,
	// y lo que se prueba acá es la deriva, no la resolución de rutas.
	call(s.handleCapture, "POST", "/api/capture", map[string]any{
		"id": "conf", "type": "architecture", "project": "x",
		"body":     "## Claim\nEl host se lee de config.yml.",
		"evidence": []map[string]string{{"kind": "file_read", "ref": archivo}},
		"check":    map[string]string{"test": "cat config.yml", "status": "not_run"},
	})
	if rec := call(s.handleVerify, "POST", "/api/verify?id=conf", nil); rec.Code != 200 {
		t.Fatalf("la primera verificación debería pasar: %d %s", rec.Code, rec.Body.String())
	}
	base, err := core.ReadNoteFile(filepath.Join(s.dir, "conf.md"))
	if err != nil {
		t.Fatal(err)
	}
	if base.Evidence[0].Hash == "" {
		t.Fatal("no se estampó la línea base de comparación")
	}

	// el archivo citado cambia: la nota ya no descansa sobre lo mismo
	if err := os.WriteFile(archivo, []byte("host: nuevo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := call(s.handleVerify, "POST", "/api/verify?id=conf", nil)
	if rec.Code != 409 {
		t.Fatalf("verificar con deriva devolvió %d; debería rechazar con 409:\n%s", rec.Code, rec.Body.String())
	}
	var r map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	if r["needs"] != "reanchor" {
		t.Errorf("la respuesta no dice cómo seguir: %v", r)
	}

	tras, _ := core.ReadNoteFile(filepath.Join(s.dir, "conf.md"))
	if tras.Evidence[0].Hash != base.Evidence[0].Hash {
		t.Error("se movió la línea base pese al rechazo: la deriva se habría borrado")
	}

	// con reanchor explícito sí procede, y recién ahí se mueve la línea base
	if rec := call(s.handleVerify, "POST", "/api/verify?id=conf&reanchor=1", nil); rec.Code != 200 {
		t.Fatalf("con reanchor debería proceder, devolvió %d: %s", rec.Code, rec.Body.String())
	}
	final, _ := core.ReadNoteFile(filepath.Join(s.dir, "conf.md"))
	if final.Evidence[0].Hash == base.Evidence[0].Hash {
		t.Error("con reanchor la línea base debería haberse actualizado")
	}
}

// 3. Cambiar lo que una nota AFIRMA cuesta la verificación, aunque el cambio
// caiga más allá de los 280 caracteres que el resumen mostraba.
func TestEdicionQueCambiaElClaimPierdeElVerde(t *testing.T) {
	s := testServer(t)
	largo := strings.Repeat("El pool de conexiones se dimensiona en 25. ", 9) // >280
	base := map[string]any{
		"id": "pool", "type": "architecture", "project": "x",
		"evidence": []map[string]string{{"kind": "file_read", "ref": "db.go:1"}},
		"check":    map[string]string{"test": "go test ./db", "status": "passed"},
	}
	color := func() string {
		var r map[string]any
		_ = json.Unmarshal(call(s.handleNote, "GET", "/api/note?id=pool", nil).Body.Bytes(), &r)
		c, _ := r["color"].(string)
		return c
	}

	base["body"] = "## Claim\n" + largo + "Y NUNCA se debe tocar sin avisar a infra."
	call(s.handleCapture, "POST", "/api/capture", base)
	call(s.handleVerify, "POST", "/api/verify?id=pool", nil)
	if c := color(); c != "green" {
		t.Fatalf("debería estar verde tras verificar, está %s", c)
	}

	// se invierte el sentido de la afirmación, después del carácter 280
	base["body"] = "## Claim\n" + largo + "Y se puede tocar libremente, sin avisar a nadie."
	call(s.handleCapture, "POST", "/api/capture", base)
	if c := color(); c == "green" {
		t.Error("AGUJERO: la nota cambió lo que afirma y conservó el verde")
	}

	// pero reformatear no cuesta nada: es el caso que cosmeticEdit protege
	call(s.handleVerify, "POST", "/api/verify?id=pool", nil)
	antes := color()
	base["body"] = "## Claim\n\n" + strings.Join(strings.Fields(largo+"Y se puede tocar libremente, sin avisar a nadie."), " ") + "\n"
	call(s.handleCapture, "POST", "/api/capture", base)
	if c := color(); c != antes {
		t.Errorf("reindentar no debería cambiar el color: %s -> %s", antes, c)
	}
}
