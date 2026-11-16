package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/importar"
	"github.com/diegoparras/cogo/internal/secretscan"
)

// cogo importar <raíz> -project <p>: arranque en frío. Lee lo que el proyecto
// ya tiene escrito y lo vuelve notas amarillas, ancladas al archivo y la línea
// de donde salieron. Ver internal/importar.
func cmdImportar(args []string) error {
	fs := flag.NewFlagSet("importar", flag.ExitOnError)
	dir := vaultFlag(fs)
	proyecto := fs.String("project", "", "proyecto de las notas (obligatorio)")
	dry := fs.Bool("dry", false, "mostrar qué se crearía, sin escribir")
	memoria := fs.String("memoria", "", "sumar la memoria que el agente ya tiene: claude, codex, o claude,codex")
	claudeDir := fs.String("claude-dir", "", "carpeta de la memoria de Claude Code (default: la de este repo en ~/.claude/projects)")
	codexHome := fs.String("codex-home", "", "home de Codex (default: $CODEX_HOME o ~/.codex)")
	_ = fs.Parse(args)
	conVault(dir)
	if fs.NArg() != 1 {
		return fmt.Errorf("uso: cogo importar <raíz del repo o carpeta de docs> -project <p> [-memoria claude,codex] [-dry]")
	}
	if strings.TrimSpace(*proyecto) == "" {
		return fmt.Errorf("importar necesita -project: las notas importadas son de un proyecto")
	}
	raiz, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if fi, err := os.Stat(raiz); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s no es una carpeta", raiz)
	}
	vault, err := core.LoadVault(*dir)
	if err != nil {
		return err
	}
	// Lo que ya está en el vault, o lo que entró antes en esta misma corrida
	// desde otra fuente, no se vuelve a crear.
	tomadas := map[string]bool{}
	existe := func(id string) bool { _, ok := vault[id]; return ok || tomadas[id] }
	opts := importar.Opciones{Proyecto: *proyecto, Hoy: today(), Existe: existe}
	notas, r, err := importar.Notas(raiz, opts)
	if err != nil {
		return err
	}
	sumar := func(ns []*core.Note, rr importar.Resumen) {
		notas = append(notas, ns...)
		r.Archivos += rr.Archivos
		r.Secciones += rr.Secciones
		for m, n := range rr.Saltadas {
			r.Saltadas[m] += n
		}
	}
	for _, n := range notas {
		tomadas[n.ID] = true
	}
	if strings.TrimSpace(*memoria) != "" {
		for _, fuente := range strings.Split(*memoria, ",") {
			var ns []*core.Note
			var rr importar.Resumen
			switch strings.TrimSpace(fuente) {
			case "claude":
				d := *claudeDir
				if d == "" {
					home, _ := os.UserHomeDir()
					d = importar.DirMemoriaClaude(home, raiz)
				}
				ns, rr, err = importar.MemoriaClaude(d, opts)
			case "codex":
				h := *codexHome
				if h == "" {
					if h = os.Getenv("CODEX_HOME"); h == "" {
						home, _ := os.UserHomeDir()
						h = filepath.Join(home, ".codex")
					}
				}
				ns, rr, err = importar.MemoriaCodex(h, opts)
				fmt.Println("ojo: la memoria de Codex es de toda la máquina, no de este repo; mirá qué entra con -dry")
			default:
				return fmt.Errorf("-memoria %q: las que COGO sabe leer son claude y codex", fuente)
			}
			if err != nil {
				return err
			}
			for _, n := range ns {
				tomadas[n.ID] = true
			}
			sumar(ns, rr)
		}
	}
	// Ninguna nota con un secreto entra al vault, venga de un documento o de una
	// memoria: es la misma regla que capture.
	limpias := notas[:0]
	for _, n := range notas {
		if resumen, bloqueada := secretscan.Nota(n); bloqueada {
			r.Saltadas["posible secreto"]++
			fmt.Printf("  no entra %s: %s\n", n.ID, resumen)
			continue
		}
		limpias = append(limpias, n)
	}
	notas = limpias
	fmt.Printf("%d archivo(s), %d sección(es): %d nota(s) nueva(s)", r.Archivos, r.Secciones, len(notas))
	if len(r.Saltadas) > 0 {
		var motivos []string
		for m, n := range r.Saltadas {
			motivos = append(motivos, fmt.Sprintf("%d %s", n, m))
		}
		sort.Strings(motivos)
		fmt.Printf(" · saltadas: %s", strings.Join(motivos, ", "))
	}
	fmt.Println()
	if *dry {
		for _, n := range notas {
			fmt.Printf("  %-12s %s  ← %s\n", n.Type, n.ID, n.Evidence[0].Ref)
		}
		return nil
	}
	if len(notas) == 0 {
		return nil
	}
	// La raíz de evidencia del proyecto: sin ella las citas `archivo:línea` no
	// resuelven, y sin resolver no hay ancla ni deriva. Solo se fija si el
	// proyecto no tenía una.
	roots := core.LoadEvidenceRoots(*dir)
	if roots.Root(*proyecto) == "" {
		prjs := roots.Projects()
		prjs[*proyecto] = raiz
		if err := core.SaveEvidenceRoots(*dir, roots.Default(), prjs); err != nil {
			return err
		}
		roots = core.LoadEvidenceRoots(*dir)
		fmt.Printf("raíz de evidencia del proyecto %q: %s\n", *proyecto, raiz)
	}
	for _, n := range notas {
		core.StampEvidenceHashes(n, roots)
		vault[n.ID] = n
	}
	core.ResolveEvidence(vault, roots)
	for _, n := range notas {
		v := core.Evaluate(n, vault, nil, today())
		n.Apply(v)
		ruta, err := core.RutaDeNota(*dir, n.ID)
		if err != nil {
			return err
		}
		if err := core.WriteNoteFile(ruta, n); err != nil {
			return err
		}
		fmt.Printf("  %s %-12s %s\n", colorTag(v.Color), n.Type, n.ID)
	}
	_ = regenIndex(*dir, vault)
	_ = appendLog(*dir, fmt.Sprintf("importar %s desde %s: %d notas", *proyecto, raiz, len(notas)))
	return nil
}
