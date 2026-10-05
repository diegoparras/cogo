package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/diegoparras/cogo/internal/purga"
)

// cogo purgar <id> [-motivo texto]
//
// Borrar de verdad: la nota de la papelera, la salida del runner en el
// registro y los artefactos que ya nadie cita. Rompe la cadena a propósito y
// lo dice. Ver internal/purga y journal.Purgar.
func cmdPurgar(args []string) error {
	fs := flag.NewFlagSet("purgar", flag.ExitOnError)
	dir := vaultFlag(fs)
	motivo := fs.String("motivo", "", "por qué (queda en el evento Purged)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cogo purgar <id> [-motivo texto] [-vault dir]")
		fs.PrintDefaults()
	}
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
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("purgar: hace falta el id de la nota")
	}
	conVault(dir)
	j, err := journalDe(*dir)
	if err != nil {
		return err
	}
	quien := os.Getenv("COGO_QUIEN")
	if quien == "" {
		quien = "cli"
	}
	res, err := purga.Purgar(context.Background(), *dir, j, pos[0], quien, *motivo)
	if err != nil {
		return err
	}
	_ = appendLog(*dir, fmt.Sprintf("purgar: %s (%s) eventos=%v archivo=%v artefactos=%d", res.Nota, quien, res.EventosReescritos, res.ArchivoBorrado, res.ArtefactosBorrados))
	fmt.Printf("purgada %s\n", res.Nota)
	if res.ArchivoBorrado {
		fmt.Println("  archivo: borrado de la papelera")
	} else {
		fmt.Println("  archivo: no estaba en la papelera (solo se limpió el registro)")
	}
	if len(res.EventosReescritos) > 0 {
		var seqs []string
		for _, s := range res.EventosReescritos {
			seqs = append(seqs, fmt.Sprint(s))
		}
		fmt.Printf("  registro: se borró la salida del runner de %d evento(s) [%s]; Purged en el evento %d\n",
			len(res.EventosReescritos), strings.Join(seqs, ", "), res.EventoPurged)
		fmt.Printf("  ATENCIÓN: la cadena queda rota A PROPÓSITO desde el evento %d. `cogo sellos` va a decir que los\n"+
			"  sellos anteriores no coinciden, y Verificar explica la purga con esta fecha. Cabeza anterior: %s\n",
			res.EventosReescritos[0], res.CabezaAnterior)
	} else {
		fmt.Printf("  registro: ningún evento llevaba salida del runner; Purged asentado en el evento %d\n", res.EventoPurged)
	}
	if res.ArtefactosBorrados > 0 {
		fmt.Printf("  artefactos: %d que ya nadie citaba\n", res.ArtefactosBorrados)
	}
	return nil
}
