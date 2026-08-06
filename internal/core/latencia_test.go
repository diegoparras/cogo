package core

import (
	"testing"
	"time"
)

// El umbral y el registro de uso se inyectan, como en producción.
func conOlvido(t *testing.T, dias int, sinConsultar map[string]int) {
	t.Helper()
	SetDiasSinConsultar(func() int { return dias })
	SetUso(func(id string, _ time.Time) time.Duration {
		return time.Duration(sinConsultar[id]) * 24 * time.Hour
	})
	t.Cleanup(func() { SetDiasSinConsultar(nil); SetUso(nil) })
}

const hoyTxt = "2026-08-04"

func vieja(id, tipo string, diasDesdeVerificada int, deps ...string) *Note {
	return &Note{
		ID: id, Type: tipo,
		LastVerified: MustDate(hoyTxt).AddDays(-diasDesdeVerificada),
		Evidence:     []Evidence{{Kind: "command_output", Ref: "x.log:1"}},
		Check:        Check{Test: "t", Status: "passed"},
		DependsOn:    deps,
		Body:         "## Claim\n" + id,
	}
}

func latencias(v map[string]*Note, contras map[string]bool) map[string]Latencia {
	return Latentes(v, contras, MustDate(hoyTxt), time.Now())
}

// El caso completo: vencida, sin dependientes, sin consultar. Sale del camino.
func TestUnaNotaMuertaSaleDelCamino(t *testing.T) {
	conOlvido(t, 180, map[string]int{"muerta": 400})
	// un `command` vence a los 30 días y expira a los 60
	v := map[string]*Note{"muerta": vieja("muerta", "command", 400)}
	l := latencias(v, nil)["muerta"]
	if !l.Latente {
		t.Fatalf("no salió del camino: %s", l.Motivo)
	}
	if l.DiasSinConsultar != 400 {
		t.Errorf("días sin consultar = %d", l.DiasSinConsultar)
	}
}

// Y las tres condiciones son necesarias, una por una.
func TestLasTresCondicionesSonNecesarias(t *testing.T) {
	t.Run("todavía no expiró", func(t *testing.T) {
		conOlvido(t, 180, map[string]int{"n": 400})
		// 40 días: ya está amarilla (venció a los 30) pero no expiró (60)
		v := map[string]*Note{"n": vieja("n", "command", 40)}
		if latencias(v, nil)["n"].Latente {
			t.Error("olvidó una nota que todavía se puede re-verificar")
		}
	})
	t.Run("alguien depende de ella", func(t *testing.T) {
		conOlvido(t, 180, map[string]int{"base": 400, "encima": 400})
		v := map[string]*Note{
			"base":   vieja("base", "command", 400),
			"encima": vieja("encima", "architecture", 10, "base"),
		}
		l := latencias(v, nil)["base"]
		if l.Latente {
			t.Error("sacó del camino una nota en la que otra se apoya")
		}
		if l.Motivo == "" {
			t.Error("no dijo quién la sostiene")
		}
	})
	t.Run("se consultó hace poco", func(t *testing.T) {
		conOlvido(t, 180, map[string]int{"n": 10})
		v := map[string]*Note{"n": vieja("n", "command", 400)}
		if latencias(v, nil)["n"].Latente {
			t.Error("olvidó una nota vencida que se consultó hace diez días — el punto entero es no olvidar por edad")
		}
	})
}

// Lo que nunca se olvida, y por qué cada uno.
func TestLoQueNuncaSaleDelCamino(t *testing.T) {
	conOlvido(t, 180, map[string]int{"n": 9999})

	t.Run("las restricciones", func(t *testing.T) {
		v := map[string]*Note{"n": vieja("n", "constraint", 3000)}
		if latencias(v, nil)["n"].Latente {
			t.Error("olvidó una restricción: son las que sostienen todo lo demás")
		}
	})
	t.Run("las fijadas", func(t *testing.T) {
		n := vieja("n", "command", 3000)
		n.Pinned = true
		if latencias(map[string]*Note{"n": n}, nil)["n"].Latente {
			t.Error("olvidó una nota fijada")
		}
	})
	t.Run("las contradichas", func(t *testing.T) {
		v := map[string]*Note{"n": vieja("n", "command", 3000)}
		if latencias(v, map[string]bool{"n": true})["n"].Latente {
			t.Error("escondió una contradicción abierta, que es justo lo que hay que ver")
		}
	})
	t.Run("las preguntas abiertas", func(t *testing.T) {
		n := vieja("n", TipoBrecha, 3000)
		n.Question = "¿se satura el pool?"
		if latencias(map[string]*Note{"n": n}, nil)["n"].Latente {
			t.Error("olvidó una pregunta abierta: que nadie la consulte no la responde")
		}
	})
}

// Con el umbral en cero no se olvida nada. Es el default de un vault que no
// configuró esto, y tiene que ser inerte.
func TestEnCeroNoSeOlvidaNada(t *testing.T) {
	conOlvido(t, 0, map[string]int{"n": 99999})
	v := map[string]*Note{"n": vieja("n", "command", 99999)}
	if latencias(v, nil)["n"].Latente {
		t.Error("olvidó con el umbral en cero")
	}
}

// Consultar una nota la despierta. Es la propiedad que hace que esto sea
// reversible sin ceremonia: la latencia se CALCULA, así que dejar de estar sin
// consultar alcanza para volver.
func TestConsultarlaLaDespierta(t *testing.T) {
	sin := map[string]int{"n": 400}
	conOlvido(t, 180, sin)
	v := map[string]*Note{"n": vieja("n", "command", 400)}
	if !latencias(v, nil)["n"].Latente {
		t.Fatal("no estaba latente para empezar")
	}
	sin["n"] = 0 // alguien la consultó
	if latencias(v, nil)["n"].Latente {
		t.Error("siguió latente después de consultarla: no hay forma de recuperarla")
	}
}

// Una nota latente sale del pack, pero el pack lo dice.
func TestElPackNoLaEntregaYLoInforma(t *testing.T) {
	conOlvido(t, 180, map[string]int{"muerta": 400, "viva": 1})
	v := map[string]*Note{
		"muerta": vieja("muerta", "command", 400),
		"viva":   vieja("viva", "architecture", 5),
	}
	p := BuildPack(v, nil, PackOptions{Today: MustDate(hoyTxt)})
	for _, id := range p.Incluidas {
		if id == "muerta" {
			t.Error("el pack entregó una nota latente")
		}
	}
	if len(p.Incluidas) != 1 || p.Incluidas[0] != "viva" {
		t.Errorf("incluidas = %v; se esperaba solo la viva", p.Incluidas)
	}
	if p.Latentes != 1 {
		t.Errorf("el pack informó %d latentes, se esperaba 1", p.Latentes)
	}
}

// El motivo siempre está, esté latente o no. Alguien que ve una nota fuera del
// camino tiene que poder saber qué la sacó — y quien ve una que sigue adentro,
// por qué se quedó.
func TestSiempreDiceElMotivo(t *testing.T) {
	conOlvido(t, 180, map[string]int{"a": 400, "b": 1})
	v := map[string]*Note{
		"a": vieja("a", "command", 400),
		"b": vieja("b", "command", 400),
	}
	for id, l := range latencias(v, nil) {
		if l.Motivo == "" {
			t.Errorf("%s no trae motivo", id)
		}
	}
}

// La búsqueda SÍ devuelve las latentes, y marcadas. Es a propósito: buscar es
// cómo se las encuentra para despertarlas, y una nota que no se puede encontrar
// no está olvidada, está perdida. Pero devolverlas sin marca haría creer que un
// agente las va a ver.
func TestLaBusquedaLasDevuelveMarcadas(t *testing.T) {
	conOlvido(t, 180, map[string]int{"muerta": 400, "viva": 1})
	v := map[string]*Note{
		"muerta": vieja("muerta", "command", 400),
		"viva":   vieja("viva", "architecture", 5),
	}
	hits := Search(v, nil, "", "", MustDate(hoyTxt), 0, false)
	if len(hits) != 2 {
		t.Fatalf("la búsqueda devolvió %d de 2: una latente que no se puede encontrar está perdida, no olvidada", len(hits))
	}
	for _, h := range hits {
		if (h.ID == "muerta") != h.Latent {
			t.Errorf("%s: latent=%v", h.ID, h.Latent)
		}
	}
}
