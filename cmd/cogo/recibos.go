package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/recibo"
	"github.com/diegoparras/cogo/internal/veredicto"
)

// El recibo epistémico: por cada `authorize`, qué sabía el agente cuando pidió.
// Ver internal/recibo.

// registrarRecibo asienta un veredicto con su contexto. Nunca falla hacia el
// llamador: un authorize que no puede escribir su recibo sigue contestando, y
// el problema queda en el log.
func registrarRecibo(ctx context.Context, d *deps, in authorizeIn, v accion.Veredicto,
	estados map[string]confidence.Estado, descartadas []string, juicios map[string]float64) string {
	r := recibo.Recibo{
		Quien: auth.CallerCtx(ctx), Accion: in.Action, Clase: v.Clase, Necesita: v.Necesita,
		Autoriza: v.Autoriza, Porque: v.Porque, Bloqueo: v.Bloqueo,
		Descartadas: descartadas, Juicios: juicios,
	}
	var seq uint64
	var cabeza string
	var ids []string
	for _, id := range in.Notes {
		ids = append(ids, id)
	}
	if j, err := journalDe(d.dir); err == nil {
		seq, cabeza = j.Cabeza()
		if evs, err := j.All(); err == nil {
			eventos := recibo.EstadosDeEventos(evs, seq, ids)
			for _, id := range ids {
				final := "desconocido"
				if e, ok := estados[id]; ok {
					final = e.String()
				}
				r.Notas = append(r.Notas, recibo.Nota{ID: id, Final: final, Eventos: eventos[id]})
			}
		}
	}
	r.Seq, r.Cabeza = seq, cabeza
	out, err := recibo.Abrir(d.dir).Agregar(r)
	if err != nil {
		_ = appendLog(d.dir, "recibo: no se pudo escribir: "+err.Error())
		return ""
	}
	return out.ID
}

// cogo recibos [-ultimo] [-accion texto] [-desde YYYY-MM-DD] [-id r-…]
func cmdRecibos(args []string) error {
	fs := flag.NewFlagSet("recibos", flag.ExitOnError)
	dir := vaultFlag(fs)
	ultimo := fs.Bool("ultimo", false, "solo el último recibo, con su reconstrucción")
	id := fs.String("id", "", "un recibo por id, con su reconstrucción")
	accionTxt := fs.String("accion", "", "filtrar por texto de la acción")
	desde := fs.String("desde", "", "desde esta fecha (YYYY-MM-DD)")
	balance := fs.Bool("balance", false, "lo que COGO evitó: bloqueos y permisos según el veredicto del humano")
	_ = fs.Parse(args)
	conVault(dir)
	if *balance {
		return balanceDeVeredictos(*dir)
	}

	st := recibo.Abrir(*dir)
	todos, err := st.Todos()
	if err != nil {
		return err
	}
	var corte time.Time
	if *desde != "" {
		d, err := core.ParseDate(*desde)
		if err != nil {
			return err
		}
		corte = d.Time()
	}
	var lista []recibo.Recibo
	for _, r := range todos {
		if *id != "" && r.ID != *id {
			continue
		}
		if *accionTxt != "" && !strings.Contains(strings.ToLower(r.Accion), strings.ToLower(*accionTxt)) {
			continue
		}
		if !corte.IsZero() && r.Cuando.Before(corte) {
			continue
		}
		lista = append(lista, r)
		if *ultimo || *id != "" {
			break
		}
	}
	if len(lista) == 0 {
		fmt.Println("sin recibos")
		return nil
	}
	j, err := journalDe(*dir)
	if err != nil {
		return err
	}
	vs, _ := veredicto.Abrir(*dir).Todos()
	for _, r := range lista {
		decision := "AUTHORIZED"
		if !r.Autoriza {
			decision = "NOT AUTHORIZED"
		}
		if v, ok := vs[r.ID]; ok {
			if v.Acertado {
				decision += "  (el humano dice: bien)"
			} else {
				decision += "  (el humano dice: MAL)"
			}
		}
		fmt.Printf("%s  %s  %s\n  %s\n  %s (%s) · pedía %s · quién: %s · registro hasta el evento %d\n",
			r.ID, r.Cuando.Local().Format("2006-01-02 15:04:05"), decision, r.Accion, r.Clase, r.Porque, r.Necesita, r.Quien, r.Seq)
		for _, n := range r.Notas {
			extra := ""
			if p, ok := r.Juicios[n.ID]; ok {
				extra = fmt.Sprintf(" · pertinencia %.2f", p)
			}
			fmt.Printf("    %-40s %s (eventos: %s)%s\n", n.ID, n.Final, n.Eventos, extra)
		}
		if len(r.Descartadas) > 0 {
			fmt.Printf("    descartadas por Jev: %s\n", strings.Join(r.Descartadas, ", "))
		}
		if r.Bloqueo != "" {
			fmt.Printf("    %s\n", strings.SplitN(r.Bloqueo, "\n", 2)[0])
		}
		if *ultimo || *id != "" {
			rec := recibo.Reconstruir(j, r)
			marca := "FIEL"
			if !rec.Fiel {
				marca = "NO FIEL"
			}
			fmt.Printf("  reconstrucción: %s — %s\n", marca, rec.Motivo)
		}
		fmt.Println()
	}
	return nil
}
