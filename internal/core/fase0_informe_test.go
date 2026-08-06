package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestFase0Informe no prueba nada: recorre el vault real con el motor vigente y
// escribe el estado de cada nota, para poder decidir la migración con datos.
// Se corre a mano con -run Fase0Informe.
func TestFase0Informe(t *testing.T) {
	dir := os.Getenv("VAULT")
	if dir == "" {
		t.Skip("sin VAULT= no hay nada que informar")
	}
	vault := map[string]*Note{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, f := range files {
		n, err := ReadNoteFile(f)
		if err != nil {
			t.Logf("  ilegible: %s (%v)", filepath.Base(f), err)
			continue
		}
		vault[n.ID] = n
	}
	ResolveEvidence(vault, EvidenceRoots{})
	hoy := MustDate("2026-08-02")
	verdicts := EvaluateVault(vault, nil, hoy)

	type fila struct {
		ID          string `json:"id"`
		Tipo        string `json:"tipo"`
		Color       string `json:"color"`
		Razon       string `json:"razon"`
		CheckStatus string `json:"check_status"`
		CheckTest   string `json:"check_test"`
		Evidencias  int    `json:"evidencias"`
		TierEv      string `json:"tier_evidencia"`
		Deps        int    `json:"deps"`
		ClaimLen    int    `json:"claim_len"`
		ClaimTrunc  bool   `json:"claim_truncado"`
		EstadoNuevo string `json:"estado_nuevo"`
	}
	var filas []fila
	for id, n := range vault {
		v := verdicts[id]
		claim := Claim(n)
		filas = append(filas, fila{
			ID: id, Tipo: n.Type, Color: v.Color.String(), Razon: v.Reason,
			CheckStatus: n.Check.Status, CheckTest: n.Check.Test,
			Evidencias: len(n.Evidence), TierEv: fmt.Sprint(evidenceTier(n.Evidence)),
			Deps: len(n.DependsOn), ClaimLen: len(claim),
			ClaimTrunc:  strings.HasSuffix(claim, "…"),
			EstadoNuevo: estadoNuevo(n, v),
		})
	}
	sort.Slice(filas, func(i, j int) bool { return filas[i].ID < filas[j].ID })

	cuenta := map[string]int{}
	colores := map[string]int{}
	trunc := 0
	for _, f := range filas {
		cuenta[f.EstadoNuevo]++
		colores[f.Color]++
		if f.ClaimTrunc {
			trunc++
		}
	}
	fmt.Println("\n════════ COLORES HOY ════════")
	for _, c := range []string{"green", "yellow", "red", "ungraded"} {
		if colores[c] > 0 {
			fmt.Printf("  %-9s %d\n", c, colores[c])
		}
	}
	fmt.Println("\n════════ ESTADO EN LA MÁQUINA NUEVA ════════")
	keys := []string{}
	for k := range cuenta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-16s %d\n", k, cuenta[k])
	}
	fmt.Printf("\n════════ EXPUESTAS AL AGUJERO (claim truncado a 280) ════════\n  %d de %d\n", trunc, len(filas))

	b, _ := json.MarshalIndent(filas, "", " ")
	out := filepath.Join("fase0", "vault-clasificado.json")
	_ = os.WriteFile(out, b, 0o644)
	fmt.Printf("\ndetalle por nota: internal/core/%s\n", out)
}

// estadoNuevo traduce el estado actual de una nota a la máquina propuesta.
func estadoNuevo(n *Note, v Verdict) string {
	switch {
	case v.Color == Ungraded:
		return "fuera_del_reticulo"
	case v.Color == Red && strings.Contains(v.Reason, "contradic"):
		return "contradicted"
	case n.Check.Status == "failed":
		return "refuted"
	case strings.Contains(v.Reason, "venc") || strings.Contains(v.Reason, "stale"):
		return "stale"
	case n.Check.Status == "passed":
		return "claimed_passed" // nadie ejecutó nada: hoy son los verdes
	case n.Check.Test != "":
		return "check_declared"
	default:
		return "asserted"
	}
}
