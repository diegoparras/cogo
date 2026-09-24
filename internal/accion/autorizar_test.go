package accion

import (
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/parametros"
)

// Un estado que el retículo no conoce bloquea, pero lo dice como lo que es: un
// problema del motor. Es el modo en que falló la primera versión del adaptador
// —convertía un uint8 a string y salía un carácter de control— y el mensaje
// culpaba a la nota.
func TestUnEstadoDesconocidoSeDenunciaComoTal(t *testing.T) {
	f := fuenteFalsa{estados: map[string]string{"n": "\x05"}}
	v := Autorizar(Peticion{Accion: "desplegar", Notas: []string{"n"}}, f, parametros.Defaults())
	if v.Autoriza {
		t.Fatal("autorizó con un estado que no existe")
	}
	if !strings.Contains(v.Falta[0].Como, "motor") {
		t.Errorf("culpó a la nota en vez de al motor: %q", v.Falta[0].Como)
	}
}

// Y el otro lado del mismo bug: los nombres que produce el retículo tienen que
// ser exactamente los que conoce el registro de parámetros. Son dos paquetes que
// no se importan entre sí, y este test es lo único que los mantiene alineados.
func TestLosNombresDeEstadoCoincidenConElReticulo(t *testing.T) {
	for _, e := range parametros.EstadosOrdenados {
		if rango(e) < 0 {
			t.Errorf("el registro conoce %q pero el autorizador no lo ubica", e)
		}
		if e != parametros.Quarantined && comoSubir[e] == "" && e != parametros.Verified {
			t.Errorf("%q no tiene un paso siguiente que ofrecer", e)
		}
	}
}
