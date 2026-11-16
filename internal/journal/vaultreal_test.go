package journal

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

// Corre la comparación sobre un vault REAL. Se saltea salvo que se le pase
// VAULT=, así que no corre en CI: es la herramienta para decidir un corte
// mirando notas de verdad y no casos de laboratorio.
func TestVaultReal(t *testing.T) {
	dir := os.Getenv("VAULT")
	if dir == "" {
		t.Skip("sin VAULT= no hay nada que comparar")
	}
	vault := map[string]*core.Note{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, f := range files {
		n, err := core.ReadNoteFile(f)
		if err != nil {
			continue
		}
		vault[n.ID] = n
	}
	core.ResolveEvidence(vault, core.EvidenceRoots{})
	hoy := core.MustDate("2026-08-03")
	verdicts := core.EvaluateVault(vault, nil, hoy)
	for id, n := range vault {
		n.StaleAt = verdicts[id].StaleAt
	}

	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sembrar(j, vault, verdicts); err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	crudos := Fold(evs, time.Time{}, time.Time{})

	local := map[string]confidence.Estado{}
	g := confidence.Grafo{}
	for id, n := range vault {
		local[id] = EstadoLocal(crudos[id], n, hoy, false)
		g[id] = n.DependsOn
	}
	final := confidence.PuntoFijo(g, local)

	ids := make([]string, 0, len(vault))
	for id := range vault {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	cambios := 0
	resumen := map[string]int{}
	t.Logf("%d notas", len(ids))
	for _, id := range ids {
		viejo, nuevo := verdicts[id].Color.String(), final[id].Color()
		if viejo == nuevo {
			continue
		}
		cambios++
		resumen[viejo+" -> "+nuevo]++
		t.Logf("  CAMBIA  %-44s %s -> %-7s  (%s)", id[:min(44, len(id))], viejo, nuevo, final[id])
		t.Logf("          razón vieja: %s", verdicts[id].Reason)
	}
	t.Logf("")
	t.Logf("cambian %d de %d notas", cambios, len(ids))
	for k, v := range resumen {
		t.Logf("   %-22s %d", k, v)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
