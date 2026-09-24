package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/jev"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/lint"
	"github.com/diegoparras/cogo/internal/suasion"
	"github.com/diegoparras/cogo/internal/xray"
)

// Jev enchufado en COGO. Cada costura sigue la misma regla: el juicio entra
// como techo, como voto que solo endurece, o como candidato para una persona.
// Con `jev.activo` apagado, sin clave, o si el servicio no responde, cada
// costura devuelve exactamente lo que COGO hacía sin Jev.

// juezDeCOGO es lo que las costuras necesitan del juez. Es una interfaz y no
// *jev.Juez para poder poner uno de mentira en los tests.
type juezDeCOGO interface {
	Disponible() bool
	Pertinencia(ctx context.Context, claim, accion string) (float64, bool)
	Clase(ctx context.Context, accion string) (string, bool)
	NivelDeEvidenciaEnCache(ref string) (string, bool)
	ChequeoEnCache(claim, test string) (float64, bool)
	Contradicen(ctx context.Context, a, b string) (float64, bool)
	Tacticas(ctx context.Context, turno string, tecnicas []jev.Tecnica) (map[string]float64, error)
	Radiografia(ctx context.Context, claim string) (compromiso, evidencia string, ok bool)
	Precalentar(ctx context.Context, notas []jev.NotaParaJuzgar) (int, error)
}

var juezDelProceso juezDeCOGO

func juezActivo() bool { return juezDelProceso != nil && juezDelProceso.Disponible() }

func umbral(clave string) float64 { return float64(pars.Entero(clave)) / 100 }

// instalarJuez arma el juez desde el entorno y engancha las costuras. Se llama
// desde instalarParametros: vale para el servidor y para la CLI por igual.
func instalarJuez(dir string) {
	j := jev.Nuevo(dir, jev.FromEnv())
	j.Activo = func() bool { return pars.Bool("jev.activo") }
	juezDelProceso = j
	engancharJuez()
}

func engancharJuez() {
	// III.3 · el nivel real de cada cita: solo caché, nunca HTTP en la
	// evaluación. Quien llena el caché es precalentarJuez, antes.
	core.SetAjusteDeTier(func(ref string, declarado core.Tier) core.Tier {
		if !juezActivo() {
			return declarado
		}
		niv, ok := juezDelProceso.NivelDeEvidenciaEnCache(ref)
		if !ok {
			return declarado
		}
		return tierDe(niv)
	})
	// III.4 · si el check prueba el claim.
	journal.SetTechoDeCheck(func(n *core.Note) confidence.Estado {
		if !juezActivo() || strings.TrimSpace(n.Check.Test) == "" {
			return confidence.Verified // sin techo
		}
		p, ok := juezDelProceso.ChequeoEnCache(core.Claim(n), n.Check.Test)
		if ok && p < umbral("jev.umbral_check") {
			return confidence.Asserted
		}
		return confidence.Verified
	})
	// III.2 · la clase por sentido.
	accion.SetClasificadorExterno(func(texto string) (accion.Clase, bool) {
		if !juezActivo() {
			return "", false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		c, ok := juezDelProceso.Clase(ctx, texto)
		if !ok {
			return "", false
		}
		return accion.Valida(c)
	})
	// III.5 · contradicciones.
	lint.SetJuezDeContradicciones(func(ctx context.Context, a, b *core.Note) (float64, bool) {
		if !juezActivo() {
			return 0, false
		}
		return juezDelProceso.Contradicen(ctx, core.Claim(a), core.Claim(b))
	}, func() float64 { return umbral("jev.umbral_contradiccion") })
}

func tierDe(nivel string) core.Tier {
	switch nivel {
	case "observed":
		return core.TierObserved
	case "reported":
		return core.TierReported
	case "reasoned":
		return core.TierReasoned
	}
	return core.TierObserved // desconocido = sin ajuste
}

// precalentarJuez llena el caché con lo que la evaluación va a leer. Se llama
// antes de evaluar, con el vault entero, y solo pregunta lo que falta.
func precalentarJuez(vault map[string]*core.Note) {
	if !juezActivo() {
		return
	}
	notas := make([]jev.NotaParaJuzgar, 0, len(vault))
	for _, n := range vault {
		item := jev.NotaParaJuzgar{Claim: core.Claim(n), Test: n.Check.Test}
		for _, e := range n.Evidence {
			item.Refs = append(item.Refs, e.Ref)
		}
		notas = append(notas, item)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if n, err := juezDelProceso.Precalentar(ctx, notas); err != nil {
		log.Printf("cogo: jev: %v (se sigue con lo que hay en caché)", err)
	} else if n > 0 {
		log.Printf("cogo: jev: %d juicios nuevos en caché", n)
	}
}

// ── III.1 · Respaldo pertinente ─────────────────────────────────────────────

// filtrarPertinentes saca del respaldo las notas que, según Jev, no hablan de
// la acción. Devuelve las que quedan y las que se fueron. Solo saca: una nota
// que Jev no puede juzgar se queda.
func filtrarPertinentes(ctx context.Context, vault map[string]*core.Note, ids []string, accionTexto string) (quedan, fuera []string) {
	if !juezActivo() {
		return ids, nil
	}
	min := umbral("jev.umbral_pertinencia")
	for _, id := range ids {
		n, ok := vault[id]
		if !ok {
			quedan = append(quedan, id)
			continue
		}
		p, ok := juezDelProceso.Pertinencia(ctx, core.Claim(n), accionTexto)
		if ok && p < min {
			fuera = append(fuera, id)
			continue
		}
		quedan = append(quedan, id)
	}
	return quedan, fuera
}

// ── III.8 · Guard y Xray ───────────────────────────────────────────────────

// juezGuard adapta el juez a lo que Guard espera.
type juezGuard struct{}

func (juezGuard) Tacticas(ctx context.Context, turn string, candidatas []*suasion.Technique) (map[string]float64, error) {
	tecs := make([]jev.Tecnica, 0, len(candidatas))
	for _, t := range candidatas {
		ej := ""
		if len(t.Detectors) > 0 {
			ej = t.Detectors[0].Signal
		}
		tecs = append(tecs, jev.Tecnica{ID: t.ID, Nombre: t.Name, EnLLM: t.InLLM, NoEs: t.FPGuard, Alias: t.AKA, Ejemplo: ej})
	}
	return juezDelProceso.Tacticas(ctx, turn, tecs)
}

// juezParaGuard devuelve el juez si está activo; nil apaga la vía.
func juezParaGuard() suasion.JuezDeTacticas {
	if !juezActivo() {
		return nil
	}
	return juezGuard{}
}

type juezXray struct{}

func (juezXray) Radiografia(ctx context.Context, claim string) (string, string, bool) {
	return juezDelProceso.Radiografia(ctx, claim)
}

func juezParaXray() xray.JuezDeClaims {
	if !juezActivo() {
		return nil
	}
	return juezXray{}
}
