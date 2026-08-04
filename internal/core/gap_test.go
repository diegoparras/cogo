package core

import (
	"strings"
	"testing"
)

func brecha(id, pregunta, costo string, bloquea ...string) *Note {
	return &Note{ID: id, Type: TipoBrecha, Question: pregunta, CostToResolve: costo,
		Blocks: bloquea, Body: "## Claim\n" + pregunta}
}

// El orden es una heurística del valor de la información: primero lo que
// destraba más decisiones; a igualdad, lo más barato de averiguar.
func TestLasBrechasSeOrdenanPorLoQueDestraban(t *testing.T) {
	vault := map[string]*Note{
		"pocas":         brecha("pocas", "¿a?", "bajo", "d1"),
		"muchas":        brecha("muchas", "¿b?", "alto", "d1", "d2", "d3"),
		"empate-caro":   brecha("empate-caro", "¿c?", "alto", "d1"),
		"empate-barato": brecha("empate-barato", "¿d?", "bajo", "d2"),
		"nota-comun":    {ID: "nota-comun", Type: "architecture", Body: "## Claim\nx"},
	}
	got := Brechas(vault)
	if len(got) != 4 {
		t.Fatalf("se esperaban 4 brechas, hay %d (¿entró una nota común?)", len(got))
	}
	if got[0].ID != "muchas" {
		t.Errorf("primero debería ir la que más destraba, fue %q", got[0].ID)
	}
	// entre las tres que destraban una sola, primero las baratas
	if got[3].ID != "empate-caro" {
		t.Errorf("la más cara debería quedar última, quedó %q", got[3].ID)
	}
}

// La relación inversa: mirando una decisión, qué preguntas la están trabando.
func TestSeSabeQuePreguntasTrabanUnaDecision(t *testing.T) {
	vault := map[string]*Note{
		"g1": brecha("g1", "¿el pool aguanta?", "medio", "migrar-db"),
		"g2": brecha("g2", "¿hay índice?", "bajo", "migrar-db", "otra"),
		"g3": brecha("g3", "¿algo más?", "bajo", "otra"),
	}
	got := BloqueadasPor(vault, "migrar-db")
	if len(got) != 2 {
		t.Fatalf("migrar-db está trabada por 2 preguntas, se encontraron %d", len(got))
	}
}

// En el pack, una brecha se muestra como PREGUNTA, no como una nota degradada.
func TestElPackMuestraLaBrechaComoPregunta(t *testing.T) {
	hoy := MustDate("2026-08-03")
	g := brecha("pool-bajo-carga", "¿El pool de conexiones se agota bajo carga sostenida?", "medio", "migrar-db", "subir-workers")
	g.Attempted = []string{"se miró el dashboard, no llega a saturar en horario normal"}
	vault := map[string]*Note{
		"pool-bajo-carga": g,
		"otra": {ID: "otra", Type: "architecture", LastVerified: hoy,
			Evidence: []Evidence{{Kind: "command_output", Ref: "x:1"}},
			Check:    Check{Test: "t", Status: "passed"}, Body: "## Claim\nalgo verificado"},
	}
	p := BuildPack(vault, nil, PackOptions{Today: hoy})

	if !strings.Contains(p.Markdown, "Open questions") {
		t.Error("el pack no tiene sección de preguntas abiertas")
	}
	if !strings.Contains(p.Markdown, "se agota bajo carga sostenida") {
		t.Error("no aparece la pregunta")
	}
	if !strings.Contains(p.Markdown, "blocks 2 decision(s)") {
		t.Errorf("no dice cuántas decisiones traba:\n%s", p.Markdown)
	}
	if !strings.Contains(p.Markdown, "already tried") {
		t.Error("no dice qué se intentó ya: el próximo va a chocar contra la misma pared")
	}
	// y NO se mezcla con los errores registrados ni con las suposiciones
	iBrechas := strings.Index(p.Markdown, "Open questions")
	iRojas := strings.Index(p.Markdown, "DO NOT RELY")
	if iRojas >= 0 && iBrechas < iRojas {
		t.Error("las preguntas abiertas aparecen antes que las suposiciones; van últimas")
	}
}

// Una brecha sin decisiones bloqueadas sigue siendo válida: registrar que algo
// no se sabe vale aunque todavía no trabe nada.
func TestUnaBrechaSinBloqueosEsValida(t *testing.T) {
	vault := map[string]*Note{"sola": brecha("sola", "¿qué pasa si...?", "")}
	p := BuildPack(vault, nil, PackOptions{Today: MustDate("2026-08-03")})
	if !strings.Contains(p.Markdown, "¿qué pasa si...?") {
		t.Errorf("una brecha sin bloqueos no apareció en el pack:\n%s", p.Markdown)
	}
}
