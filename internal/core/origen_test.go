package core

import (
	"strings"
	"testing"
)

func nota(tipo, origen string) *Note {
	return &Note{
		ID: "n", Type: tipo, Origin: origen, LastVerified: MustDate("2026-08-04"),
		Evidence: []Evidence{{Kind: "command_output", Ref: "x.log:1"}},
		Check:    Check{Test: "t", Status: "passed"},
		Body:     "## Claim\nalgo",
	}
}

// La decisión de diseño, y la que más fácil sería revertir sin querer: el origen
// NO baja el color. Se evaluó ponerle techo y se descartó — obligaría a ratificar
// a mano cada decisión que tome un agente.
func TestElOrigenNoBajaElColor(t *testing.T) {
	hoy := MustDate("2026-08-04")
	for _, o := range []string{"agent", "human", "instrument", ""} {
		n := nota("decision", o)
		v := EvaluateVault(map[string]*Note{n.ID: n}, nil, hoy)[n.ID]
		if v.Color != Green {
			t.Errorf("origen %q dejó la nota en %s: %s", o, v.Color, v.Reason)
		}
	}
}

// Pero sí se dice, y solo donde hace falta.
func TestSoloLasNormativasDeclaranOrigen(t *testing.T) {
	for _, tipo := range []string{"decision", "constraint"} {
		if !EsNormativa(nota(tipo, "agent")) {
			t.Errorf("%q tendría que ser normativa", tipo)
		}
	}
	for _, tipo := range []string{"bug", "runbook", "architecture", "command"} {
		n := nota(tipo, "agent")
		if EsNormativa(n) {
			t.Errorf("%q no es normativa: la evidencia ya responde por ella", tipo)
		}
		if lineaOrigen(n) != "" {
			t.Errorf("%q mostró origen en el pack: es ruido", tipo)
		}
	}
}

// Lo que el agente lee. La frase importa tanto como el campo: "proposed by an
// agent" le dice que puede discutirlo; sin eso lo trata como resuelto.
func TestElPackDiceQuienDecidio(t *testing.T) {
	casos := map[string]string{
		"agent":      "proposed by an agent",
		"human":      "decided by a human",
		"instrument": "measured, not chosen",
		"":           "unrecorded",
	}
	for origen, esperado := range casos {
		got := lineaOrigen(nota("decision", origen))
		if !strings.Contains(got, esperado) {
			t.Errorf("origen %q rindió %q; se esperaba que dijera %q", origen, got, esperado)
		}
	}
}

// Un valor inventado cae en "agente": quien captura ES un agente, así que es la
// suposición conservadora, y un typo no puede ser una forma de que una propuesta
// pase por decisión.
func TestUnOrigenInventadoEsDelAgente(t *testing.T) {
	for _, malo := range []string{"humano", "HUMAN!", "boss", "el equipo"} {
		if got := NormalizarOrigen(malo); got != OrigenAgente {
			t.Errorf("NormalizarOrigen(%q) = %q, se esperaba %q", malo, got, OrigenAgente)
		}
	}
	// Pero vacío NO es del agente: es "no consta", que es lo que tienen las
	// notas escritas antes de que el campo existiera. Confundirlos borraría la
	// diferencia entre no haber podido declarar y no haber querido.
	if got := NormalizarOrigen("  "); got != OrigenSinDeclarar {
		t.Errorf("un origen vacío dio %q; tiene que quedar sin declarar", got)
	}
}

// Las notas viejas quedan señaladas, no reclasificadas.
func TestLasNotasViejasQuedanSinDeclarar(t *testing.T) {
	vieja := nota("decision", "")
	if !SinDeclarar(vieja) {
		t.Error("una decisión sin campo origin tendría que quedar marcada como sin declarar")
	}
	if EsPropuesta(vieja) {
		t.Error("una nota vieja se dio por propuesta del agente: eso es reescribir el pasado")
	}
	if SinDeclarar(nota("bug", "")) {
		t.Error("un bug sin origen no es un dato faltante: no le corresponde")
	}
}

// El origen sobrevive el viaje por el frontmatter.
func TestElOrigenSobreviveElArchivo(t *testing.T) {
	n := nota("decision", "human")
	b, err := MarshalNote(n)
	if err != nil {
		t.Fatal(err)
	}
	leida, err := ParseNote(b)
	if err != nil {
		t.Fatal(err)
	}
	if leida.Origin != "human" {
		t.Errorf("el origen no sobrevivió: %q\n%s", leida.Origin, b)
	}
}
