package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/diegoparras/cogo/internal/runner"
)

// La sala de guerra: el estado del motor, no sus perillas.
//
// # POR QUÉ ES OTRA COSA QUE EL PANEL DE PARÁMETROS
//
// Veintiún controles son un panel de configuración. Lo que lo convierte en una
// sala de guerra es lo que NO se toca: qué está pasando ahora mismo.
//
// Y lo más importante que faltaba mostrar es el registro de eventos. Es,
// literalmente, el recibo de cada color que COGO calcula: la cadena encadenada
// por hash de la que se pliega todo. Vivía en `.cogo/journal/*.jsonl` y no había
// forma de verlo sin abrir el archivo — el corazón del motor, invisible.
//
// # LOS OCHO ESTADOS, NO LOS TRES COLORES
//
// El Vault muestra verde, amarillo y rojo porque es lo que un humano necesita de
// un vistazo. Acá corresponde la verdad completa: el color es una PROYECCIÓN de
// ocho estados sobre tres, y esa proyección esconde justo la distinción que más
// importa — cuántas notas están en `claimed_passed` esperando que alguien
// ejecute el check, contra cuántas llegaron a `verified` de verdad.
//
// Las dos son verdes. No son lo mismo.

type eventoVista struct {
	Seq    uint64 `json:"seq"`
	Cuando string `json:"cuando"`
	Nota   string `json:"nota"`
	Tipo   string `json:"tipo"`
	Emisor string `json:"emisor"`
	Guarda string `json:"guarda,omitempty"`
}

type conteo struct {
	Nombre string `json:"nombre"`
	N      int    `json:"n"`
	Color  string `json:"color,omitempty"`
	Nota   string `json:"nota,omitempty"`
}

func (s *Server) handleSalaGuerra(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	vault, err := s.cache.Load()
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	core.ResolveEvidence(vault, s.evRoots())
	contras := s.contras()
	hoy := s.today()

	// ── el registro ────────────────────────────────────────────────────────
	if j, err := s.journal(); err == nil {
		if evs, err := j.All(); err == nil {
			out["registro"] = s.vistaRegistro(j, evs)
			out["reticulo"] = vistaReticulo(vault, contras, hoy, evs)
			out["runner"] = s.vistaRunner(evs)
		}
	}

	// ── el vault de un vistazo ─────────────────────────────────────────────
	lat := core.Latentes(vault, contras, hoy, time.Now())
	var latentes, fijadas, brechas, propuestas, sinOrigen int
	for id, n := range vault {
		if lat[id].Latente {
			latentes++
		}
		if n.Pinned {
			fijadas++
		}
		if core.EsBrecha(n) {
			brechas++
		}
		if core.EsPropuesta(n) {
			propuestas++
		}
		if core.SinDeclarar(n) {
			sinOrigen++
		}
	}
	out["vault"] = map[string]any{
		"notas": len(vault), "latentes": latentes, "fijadas": fijadas,
		"brechas": brechas, "propuestas": propuestas, "sin_origen": sinOrigen,
	}

	out["grafo"] = saludDelGrafo(vault)
	out["autorizaciones"] = s.vistaAutorizaciones()
	writeJSON(w, out)
}

// vistaRegistro arma lo que se muestra del journal: su integridad primero.
//
// La integridad va arriba de todo y no enterrada en una estadística: una cadena
// rota significa que alguien editó un evento pasado, y a partir de ahí NADA de
// lo que COGO diga sobre confianza vale. Es el único dato de esta pantalla que
// invalida a todos los demás.
func (s *Server) vistaRegistro(j *journal.Journal, evs []journal.Event) map[string]any {
	integra, problema := true, ""
	if err := j.Verificar(); err != nil {
		integra, problema = false, err.Error()
	}

	porTipo := map[string]int{}
	porEmisor := map[string]int{}
	for _, e := range evs {
		porTipo[e.Kind]++
		if q := strings.TrimSpace(e.Emitter); q != "" {
			porEmisor[q]++
		}
	}

	// Los últimos primero: en una sala de guerra se mira lo que acaba de pasar.
	const cuantos = 60
	desde := 0
	if len(evs) > cuantos {
		desde = len(evs) - cuantos
	}
	ultimos := make([]eventoVista, 0, cuantos)
	for i := len(evs) - 1; i >= desde; i-- {
		e := evs[i]
		cuando := e.ValidTime
		if cuando.IsZero() {
			cuando = e.TxTime
		}
		ultimos = append(ultimos, eventoVista{
			Seq: e.Seq, Cuando: cuando.UTC().Format(time.RFC3339),
			Nota: e.NoteID, Tipo: e.Kind, Emisor: e.Emitter, Guarda: e.Guard,
		})
	}
	return map[string]any{
		"total": len(evs), "integra": integra, "problema": problema,
		"ultimos": ultimos, "por_tipo": aConteos(porTipo), "por_emisor": aConteos(porEmisor),
	}
}

// vistaReticulo cuenta las notas por estado del retículo, en su orden.
func vistaReticulo(vault map[string]*core.Note, contras map[string]bool, hoy core.Date, evs []journal.Event) []conteo {
	final, _ := motor.Estados(vault, contras, hoy, evs)
	n := map[confidence.Estado]int{}
	for _, e := range final {
		n[e]++
	}
	orden := []confidence.Estado{
		confidence.Quarantined, confidence.Refuted, confidence.Contradicted, confidence.Stale,
		confidence.Asserted, confidence.CheckDeclared, confidence.ClaimedPassed, confidence.Verified,
	}
	out := make([]conteo, 0, len(orden))
	for _, e := range orden {
		out = append(out, conteo{Nombre: e.String(), N: n[e], Color: e.Color()})
	}
	return out
}

// vistaRunner cruza los checks que el vault autoriza con lo que realmente se
// ejecutó. La diferencia entre las dos columnas es la que importa: un check
// declarado y jamás corrido es una promesa, no una verificación.
func (s *Server) vistaRunner(evs []journal.Event) map[string]any {
	cfg, err := runner.Cargar(s.dir)
	if err != nil || cfg == nil {
		return map[string]any{"habilitado": false, "error": errTexto(err)}
	}
	type corrida struct {
		Cuando string `json:"cuando"`
		Nota   string `json:"nota"`
		OK     bool   `json:"ok"`
	}
	ultima := map[string]corrida{}
	ok, fallos := 0, 0
	for _, e := range evs {
		if e.Kind != "CheckExecuted" {
			continue
		}
		var p struct {
			Check string `json:"check"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		bien := e.Guard != "ejecucion_falla"
		if bien {
			ok++
		} else {
			fallos++
		}
		cuando := e.ValidTime
		if cuando.IsZero() {
			cuando = e.TxTime
		}
		ultima[p.Check] = corrida{Cuando: cuando.UTC().Format(time.RFC3339), Nota: e.NoteID, OK: bien}
	}
	type chk struct {
		ID      string   `json:"id"`
		Comando []string `json:"comando"`
		Workdir string   `json:"workdir"`
		Ultima  *corrida `json:"ultima,omitempty"`
	}
	var checks []chk
	for _, c := range cfg.Checks {
		x := chk{ID: c.ID, Comando: c.Comando, Workdir: c.Workdir}
		if u, hay := ultima[c.ID]; hay {
			x.Ultima = &u
		}
		checks = append(checks, x)
	}
	return map[string]any{
		"habilitado": cfg.Habilitado, "checks": checks,
		"ejecuciones_ok": ok, "ejecuciones_falladas": fallos,
	}
}

// saludDelGrafo busca lo que rompe la propagación: ciclos y dependencias a notas
// que no existen.
//
// Las dos se ven como un rojo cualquiera en el Vault, y tienen arreglos
// distintos: una dependencia rota se corrige apuntando bien, un ciclo se corta
// decidiendo cuál de las dos notas es la primera. Distinguirlas acá ahorra la
// media hora de averiguar cuál de las dos es.
func saludDelGrafo(vault map[string]*core.Note) map[string]any {
	var faltantes []conteo
	for id, n := range vault {
		for _, d := range n.DependsOn {
			if _, existe := vault[d]; !existe {
				faltantes = append(faltantes, conteo{Nombre: d, Nota: id})
			}
		}
	}
	sort.Slice(faltantes, func(i, j int) bool { return faltantes[i].Nota < faltantes[j].Nota })

	// Ciclos: DFS con marca de "en camino". Se reporta uno por componente, que
	// es lo que hace falta para ir a arreglarlo.
	estado := map[string]int{} // 0 sin ver, 1 en camino, 2 cerrado
	var ciclos [][]string
	var camino []string
	var visitar func(string)
	visitar = func(id string) {
		estado[id] = 1
		camino = append(camino, id)
		if n, ok := vault[id]; ok {
			for _, d := range n.DependsOn {
				switch estado[d] {
				case 0:
					if _, existe := vault[d]; existe {
						visitar(d)
					}
				case 1:
					desde := 0
					for i, x := range camino {
						if x == d {
							desde = i
							break
						}
					}
					ciclos = append(ciclos, append([]string(nil), camino[desde:]...))
				}
			}
		}
		camino = camino[:len(camino)-1]
		estado[id] = 2
	}
	ids := make([]string, 0, len(vault))
	for id := range vault {
		ids = append(ids, id)
	}
	sort.Strings(ids) // el orden estable hace que el reporte no baile entre recargas
	for _, id := range ids {
		if estado[id] == 0 {
			visitar(id)
		}
	}
	return map[string]any{"faltantes": faltantes, "ciclos": ciclos}
}

// vistaAutorizaciones lee lo que `authorize` fue dejando en el log del vault.
//
// Toda consulta queda registrada, autorice o no. Un control que solo deja rastro
// cuando bloquea no sirve para auditar: lo que uno quiere poder reconstruir es
// en qué se apoyó cada acción, sobre todo las que pasaron.
func (s *Server) vistaAutorizaciones() map[string]any {
	f, err := os.Open(filepath.Join(s.dir, "log.md"))
	if err != nil {
		return map[string]any{"total": 0}
	}
	defer f.Close()

	type fila struct {
		Cuando   string `json:"cuando"`
		Clase    string `json:"clase"`
		Necesita string `json:"necesita"`
		Autoriza bool   `json:"autoriza"`
		Notas    string `json:"notas"`
	}
	var filas []fila
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		ln := sc.Text()
		i := strings.Index(ln, " authorize ")
		if !strings.HasPrefix(ln, "- ") || i < 0 {
			continue
		}
		cuando := strings.TrimSpace(ln[2:i])
		resto := ln[i+len(" authorize "):]
		// "irreversible [verified] [id1 id2] -> false"
		autoriza := strings.HasSuffix(resto, "true")
		clase, necesita, notas := resto, "", ""
		if a := strings.Index(resto, " ["); a >= 0 {
			clase = resto[:a]
			if b := strings.Index(resto[a:], "]"); b >= 0 {
				necesita = resto[a+2 : a+b]
				if c := strings.Index(resto[a+b:], "["); c >= 0 {
					if d := strings.Index(resto[a+b+c:], "]"); d >= 0 {
						notas = resto[a+b+c+1 : a+b+c+d]
					}
				}
			}
		}
		filas = append(filas, fila{Cuando: cuando, Clase: clase, Necesita: necesita,
			Autoriza: autoriza, Notas: notas})
	}
	bloqueadas := 0
	for _, f := range filas {
		if !f.Autoriza {
			bloqueadas++
		}
	}
	// Las últimas primero, y acotadas: el log completo puede ser enorme.
	sort.SliceStable(filas, func(i, j int) bool { return filas[i].Cuando > filas[j].Cuando })
	if len(filas) > 40 {
		filas = filas[:40]
	}
	return map[string]any{"total": len(filas), "bloqueadas": bloqueadas, "ultimas": filas}
}

func aConteos(m map[string]int) []conteo {
	out := make([]conteo, 0, len(m))
	for k, v := range m {
		out = append(out, conteo{Nombre: k, N: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Nombre < out[j].Nombre
	})
	return out
}

func errTexto(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
