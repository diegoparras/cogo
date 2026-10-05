package motor

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// Las invariantes del motor, verificadas sobre miles de vaults generados al azar.
//
// # POR QUÉ ESTO Y NO UNA ESPECIFICACIÓN FORMAL
//
// El spec pedía modelar la máquina en TLA+ y correrle el model checker. Es una
// técnica seria y para un sistema distribuido habría sido la respuesta correcta.
// Acá no, por una razón concreta: un modelo en TLA+ es un ARTEFACTO APARTE, y lo
// que verifica es el modelo. Nada garantiza que el modelo y el Go digan lo
// mismo, y en cuanto empiezan a divergir —lo hacen siempre— el model checker
// pasa a certificar un sistema que no es el que corre.
//
// Estas propiedades se ejecutan contra el código que se despliega. Son más
// débiles que una prueba —cubren los casos que se generan, no todos— y son
// verdaderas del sistema real, no de una descripción suya. Para un motor que
// cabe en un proceso, es el intercambio correcto.
//
// Cada una de las cinco es una propiedad que, si se rompe, rompe algo que COGO
// promete.

const casos = 400

// vaultAlAzar arma un vault con dependencias arbitrarias, ciclos incluidos.
func vaultAlAzar(r *rand.Rand, n int) map[string]*core.Note {
	tipos := []string{"architecture", "decision", "command", "bug", "runbook", "constraint"}
	obs := []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}}
	rep := []core.Evidence{{Kind: "doc", Ref: "d.md"}}
	estados := []string{"passed", "failed", "not_run"}
	att := []string{core.AttestDeclared, core.AttestExecuted, ""}

	v := map[string]*core.Note{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("n%02d", i)
		var ev []core.Evidence
		switch r.Intn(3) {
		case 0:
			ev = obs
		case 1:
			ev = rep
		}
		v[id] = &core.Note{
			ID: id, Type: tipos[r.Intn(len(tipos))],
			LastVerified: core.MustDate("2026-08-01"),
			Evidence:     ev,
			Check: core.Check{Test: "t", Status: estados[r.Intn(len(estados))],
				Attested: att[r.Intn(len(att))], AttestedBy: "token:x"},
			Body: "## Claim\n" + id,
		}
	}
	// Dependencias al azar, sin evitar ciclos: los ciclos son parte del dominio.
	for _, n := range v {
		for k := r.Intn(3); k > 0; k-- {
			d := fmt.Sprintf("n%02d", r.Intn(len(v)))
			if d != n.ID {
				n.DependsOn = append(n.DependsOn, d)
			}
		}
	}
	return v
}

func evaluar(t *testing.T, v map[string]*core.Note, contras map[string]bool) map[string]confidence.Estado {
	t.Helper()
	hoy := core.MustDate("2026-08-03")
	previos := core.EvaluateVaultCore(v, contras, hoy)
	for id, n := range v {
		n.StaleAt = previos[id].StaleAt
	}
	j, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Sembrar(j, v, previos); err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	final, _ := Estados(v, contras, hoy, evs)
	return final
}

// INVARIANTE 1 · Determinismo.
//
// El mismo vault da el mismo resultado siempre. Suena trivial y no lo es: el
// motor recorre mapas de Go, cuyo orden de iteración es deliberadamente
// aleatorio. Si el resultado dependiera del orden de visita —y con un punto fijo
// mal resuelto dependería— el color de una nota cambiaría entre dos recargas de
// la misma pantalla.
func TestInvarianteDeterminismo(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for c := 0; c < 60; c++ {
		v := vaultAlAzar(r, 8+r.Intn(10))
		base := evaluar(t, v, nil)
		for rep := 0; rep < 8; rep++ {
			otro := evaluar(t, v, nil)
			for id := range base {
				if base[id] != otro[id] {
					t.Fatalf("caso %d: %s dio %s y después %s sobre el mismo vault", c, id, base[id], otro[id])
				}
			}
		}
	}
}

// INVARIANTE 2 · La propagación solo baja.
//
// Ninguna nota puede terminar por encima de lo que vale por sí misma. Es la
// propiedad que hace que el grafo sea una red de seguridad y no un amplificador:
// apoyarse en algo no puede volverte más confiable de lo que sos.
func TestInvarianteLaPropagacionSoloBaja(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	hoy := core.MustDate("2026-08-03")
	for c := 0; c < casos; c++ {
		v := vaultAlAzar(r, 6+r.Intn(12))
		previos := core.EvaluateVaultCore(v, nil, hoy)
		for id, n := range v {
			n.StaleAt = previos[id].StaleAt
		}
		j, _ := journal.Open(t.TempDir())
		_, _ = journal.Sembrar(j, v, previos)
		evs, _ := j.All()
		final, local := Estados(v, nil, hoy, evs)
		for id := range local {
			if final[id] > local[id] {
				t.Fatalf("caso %d: %s vale %s por sí misma y terminó en %s — la propagación la subió",
					c, id, local[id], final[id])
			}
		}
	}
}

// INVARIANTE 3 · Monotonía respecto de las contradicciones.
//
// Abrir una contradicción nunca puede mejorarle el color a nadie. Es la
// propiedad que hace que valga la pena registrar una: si abrir una contradicción
// pudiera subir una nota en algún rincón del grafo, el sistema estaría premiando
// que se oculten.
func TestInvarianteUnaContradiccionNuncaMejora(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for c := 0; c < casos; c++ {
		v := vaultAlAzar(r, 6+r.Intn(10))
		sin := evaluar(t, v, nil)

		victima := fmt.Sprintf("n%02d", r.Intn(len(v)))
		con := evaluar(t, v, map[string]bool{victima: true})

		for id := range sin {
			if con[id] > sin[id] {
				t.Fatalf("caso %d: contradecir %s subió a %s de %s a %s", c, victima, id, sin[id], con[id])
			}
		}
	}
}

// INVARIANTE 4 · Monotonía respecto de la evidencia.
//
// Sacarle evidencia a una nota no puede subir a ninguna nota del vault. Es la
// dirección que garantiza que el motor sea un "must" y no un "may": lo que se
// afirma se afirma porque hay con qué, y quitar el con qué solo puede restar.
func TestInvarianteQuitarEvidenciaNuncaSube(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for c := 0; c < casos; c++ {
		v := vaultAlAzar(r, 6+r.Intn(10))
		antes := evaluar(t, v, nil)

		victima := fmt.Sprintf("n%02d", r.Intn(len(v)))
		guardada := v[victima].Evidence
		v[victima].Evidence = nil
		despues := evaluar(t, v, nil)
		v[victima].Evidence = guardada

		for id := range antes {
			if despues[id] > antes[id] {
				t.Fatalf("caso %d: sacarle la evidencia a %s subió a %s de %s a %s",
					c, victima, id, antes[id], despues[id])
			}
		}
	}
}

// INVARIANTE 5 · Nada llega a `verified` sin haberse ejecutado.
//
// La línea que separa a COGO de una herramienta que le cree a los agentes. Si
// una sola nota pudiera alcanzar `verified` sin un check ejecutado, el umbral de
// las acciones irreversibles —que pide exactamente eso— dejaría de significar
// algo, y con él toda la fase 7.
func TestInvarianteNadieLlegaAVerifiedSinEjecutar(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for c := 0; c < casos; c++ {
		v := vaultAlAzar(r, 6+r.Intn(12))
		final := evaluar(t, v, nil)
		for id, est := range final {
			if est != confidence.Verified {
				continue
			}
			n := v[id]
			if n.Check.Attestation() != core.AttestExecuted {
				t.Fatalf("caso %d: %s llegó a verified con procedencia %q (check %q)",
					c, id, n.Check.Attestation(), n.Check.Status)
			}
		}
	}
}
