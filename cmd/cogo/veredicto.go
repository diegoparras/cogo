package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/diegoparras/cogo/internal/recibo"
	"github.com/diegoparras/cogo/internal/veredicto"
)

// cogo veredicto <recibo> bien|mal [-nota texto]
//
// Lo que el humano dice de una decisión de COGO. Es la única fuente de "lo que
// COGO evitó": un bloqueo sin veredicto no cuenta a favor ni en contra.
func cmdVeredicto(args []string) error {
	fs := flag.NewFlagSet("veredicto", flag.ExitOnError)
	dir := vaultFlag(fs)
	nota := fs.String("nota", "", "por qué (opcional)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cogo veredicto <recibo-id> bien|mal [-nota texto] [-vault dir]")
		fs.PrintDefaults()
	}
	// Los posicionales pueden venir antes o después de las banderas.
	var pos []string
	rest := args
	for len(rest) > 0 {
		_ = fs.Parse(rest)
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		rest = rest[1:]
	}
	if len(pos) != 2 {
		fs.Usage()
		return fmt.Errorf("veredicto: hacen falta el id del recibo y bien|mal")
	}
	id, dicho := pos[0], strings.ToLower(pos[1])
	var acertado bool
	switch dicho {
	case "bien", "si", "sí", "ok", "acerto", "acertó":
		acertado = true
	case "mal", "no", "erro", "erró":
		acertado = false
	default:
		return fmt.Errorf("veredicto: %q no es bien ni mal", pos[1])
	}
	conVault(dir)
	if _, ok := recibo.Abrir(*dir).Buscar(id); !ok {
		return fmt.Errorf("veredicto: no hay recibo %s", id)
	}
	quien := os.Getenv("COGO_QUIEN")
	if quien == "" {
		quien = "cli"
	}
	v := veredicto.Veredicto{Recibo: id, Acertado: acertado, Nota: strings.TrimSpace(*nota), Quien: quien}
	if err := veredicto.Abrir(*dir).Poner(v); err != nil {
		return err
	}
	_ = appendLog(*dir, fmt.Sprintf("veredicto: %s %s (%s)", id, dicho, quien))
	fmt.Printf("%s: COGO %s\n", id, map[bool]string{true: "tenía razón", false: "se equivocó"}[acertado])
	return nil
}

// balanceDeVeredictos imprime lo que COGO evitó, según el humano.
func balanceDeVeredictos(dir string) error {
	todos, err := recibo.Abrir(dir).Todos()
	if err != nil {
		return err
	}
	autorizo := map[string]bool{}
	for _, r := range todos {
		autorizo[r.ID] = r.Autoriza
	}
	vs, err := veredicto.Abrir(dir).Todos()
	if err != nil {
		return err
	}
	b := veredicto.Balancear(autorizo, vs)
	fmt.Printf("decisiones: %d · con veredicto: %d · sin juzgar: %d\n", b.TotalDecidido, b.Juzgados, b.SinJuzgar)
	fmt.Printf("  bloqueos acertados: %d   bloqueos de más: %d\n", b.BloqueosBien, b.BloqueosMal)
	fmt.Printf("  permisos acertados: %d   permisos de más: %d  <- el peor caso\n", b.PermisosBien, b.PermisosMal)
	return nil
}
