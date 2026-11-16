package accion

import "testing"

// Cada señal tiene que reconocer su propio ejemplo.
//
// Es el test más barato del paquete y el que más habría ahorrado: `rm\s+-[rf]`
// con un \b al final dejó de reconocer "rm -rf" —la clase de caracteres toma UNA
// letra, y la "f" que sigue impide el límite de palabra— y el sistema no dio
// ningún error. Autorizó un borrado como si fuera una respuesta.
//
// Un patrón que no reconoce lo que dice reconocer no es un bug de estilo: es un
// control apagado que se ve encendido.
func TestCadaSeñalReconoceSuEjemplo(t *testing.T) {
	for _, s := range señales {
		if s.ejemplo == "" {
			t.Errorf("la señal %q (%s) no declara ejemplo", s.que, s.clase)
			continue
		}
		if !s.re.MatchString(s.ejemplo) {
			t.Errorf("la señal %q no reconoce su propio ejemplo %q\n  patrón: %s", s.que, s.ejemplo, s.re)
		}
		// Y tiene que clasificar en la clase que declara, no en una más suave por
		// culpa de otra señal que también matchee.
		if got := Clasificar(s.ejemplo); severidad(got.Clase) < severidad(s.clase) {
			t.Errorf("%q clasificó como %q y la señal %q es %q", s.ejemplo, got.Clase, s.que, s.clase)
		}
	}
}

// Las formas de borrar que más aparecen en la vida real, uno por uno.
func TestLosBorradosDeVerdadSeReconocen(t *testing.T) {
	for _, texto := range []string{
		"limpiar unos temporales con rm -rf en la carpeta de build",
		"rm -r ./dist",
		"correr DROP TABLE usuarios en la base de staging",
		"git push --force a la rama principal",
		"un git reset --hard para volver atrás",
		"Remove-Item -Recurse el directorio viejo",
		"del /s /q la carpeta de logs",
	} {
		if inf := Clasificar(texto); inf.Clase != Irreversible {
			t.Errorf("%q clasificó como %q, no como irreversible", texto, inf.Clase)
		}
	}
}
