package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/ancla"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// cmdSellar publica la cabeza del registro afuera.
//
// El comando hace poco: pide la cabeza, la manda al destino configurado y la
// anota. Lo que importa es lo que NO hace — no inventa un destino, y con el
// destino manual no publica nada: imprime el sello y le dice a la persona que lo
// publique. Un sello que COGO se manda a sí mismo no prueba nada contra COGO.
func cmdSellar(args []string) error {
	fs := flag.NewFlagSet("sellar", flag.ExitOnError)
	dir := vaultFlag(fs)
	nota := fs.String("nota", "", "dónde lo publicaste, para poder encontrarlo después")
	_ = fs.Parse(args)
	conVault(dir)

	j, err := registroAlDia(*dir)
	if err != nil {
		return err
	}
	seq, cabeza := j.Cabeza()
	if seq == 0 || cabeza == "" {
		return fmt.Errorf("el registro está vacío: no hay nada que sellar todavía")
	}

	destino := pars.Texto("sello.destino")
	url := pars.Texto("sello.url")
	recibo, err := ancla.Publicar(destino, url, seq, cabeza)
	if err != nil {
		return err
	}

	donde := destino
	if destino == ancla.DestinoHTTPS {
		donde = url
	}
	s := ancla.Sello{Seq: seq, Cabeza: cabeza, Donde: donde, Recibo: recibo, Nota: *nota}
	if err := ancla.Abrir(*dir).Agregar(s); err != nil {
		return err
	}

	if destino == ancla.DestinoManual || destino == "" {
		fmt.Print(ancla.ComoPublicarloAMano(seq, cabeza))
		if strings.TrimSpace(*nota) == "" {
			fmt.Println("\nCuando lo publiques, anotá dónde:")
			fmt.Println("  cogo sellar -nota \"commit a1b2c3 del repo de actas\"")
		}
		return nil
	}
	fmt.Printf("sellado el evento %d en %s\n", seq, donde)
	if recibo != "" {
		fmt.Printf("  recibo: %s\n", recibo)
	}
	return nil
}

// cmdVerificarSellos contrasta cada sello contra el registro de hoy.
//
// Es la mitad que le da sentido a la otra: sellar sin poder verificar es
// juntar hashes.
func cmdVerificarSellos(args []string) error {
	fs := flag.NewFlagSet("sellos", flag.ExitOnError)
	dir := vaultFlag(fs)
	_ = fs.Parse(args)
	conVault(dir)

	j, err := registroAlDia(*dir)
	if err != nil {
		return err
	}
	sellos, err := ancla.Abrir(*dir).Todos()
	if err != nil {
		return err
	}
	if len(sellos) == 0 {
		fmt.Println("no hay sellos todavía. `cogo sellar` publica el primero.")
		return nil
	}

	malos := 0
	for _, r := range ancla.Verificar(sellos, j.DigestDe) {
		marca := "ok "
		if !r.OK {
			marca = "MAL"
			malos++
		}
		fmt.Printf("%s  evento %-7d %s\n", marca, r.Sello.Seq, r.Dice)
	}
	if malos > 0 {
		return fmt.Errorf("%d de %d sellos no coinciden: el registro cambió después de publicarlos", malos, len(sellos))
	}
	fmt.Printf("\n%d sellos, todos coinciden con el registro actual.\n", len(sellos))
	return nil
}

// registroAlDia abre el registro y se asegura de que conozca las notas del
// vault.
//
// Hace falta porque el registro se siembra en la primera evaluación, y esa
// evaluación solo ocurre corriendo el servidor. Un vault que nunca se sirvió
// tiene notas y un registro vacío — y sellar un registro vacío no sella nada.
//
// Sembrar acá es lo mismo que hace el servidor al arrancar, y es idempotente.
func registroAlDia(dir string) (*journal.Journal, error) {
	if err := instalarMotor(dir); err != nil {
		return nil, err
	}
	vault, err := core.LoadVault(dir)
	if err != nil {
		return nil, err
	}
	core.EvaluateVault(vault, nil, today()) // dispara la siembra
	return journalDe(dir)
}
