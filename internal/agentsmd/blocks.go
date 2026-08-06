package agentsmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// A Block is one reusable piece of an agent instruction file. The point is that
// the canonical wording lives HERE, curated and versioned with the protocol,
// instead of being retyped from memory into every AGENTS.md / CLAUDE.md /
// GEMINI.md. The visor lists them as chips and composes a file with clicks.
type Block struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Desc      string `json:"desc"`             // one line: why you'd include it
	Essential bool   `json:"essential"`        // the must-haves for any agent
	Needs     string `json:"needs,omitempty"`  // "project" | "token": the visor asks for it
	Markdown  string `json:"markdown"`         // the text inserted
	Custom    bool   `json:"custom,omitempty"` // true for the user's own blocks
}

// BlockOptions parameterizes the curated blocks that need context.
type BlockOptions struct {
	HTTPURL string // MCP endpoint for the connection block
	Project string // project name for the project block
	Token   string // optional bearer token to embed in the connection snippet
}

// Curated returns COGO's recommended blocks, already filled with the given
// context. Order matters: it's the order a sensible file would read in.
func Curated(o BlockOptions) []Block {
	proj := strings.TrimSpace(o.Project)
	if proj == "" {
		proj = "TU-PROYECTO"
	}
	return []Block{
		{
			ID: "que-es-cogo", Title: "Qué es COGO", Essential: true,
			Desc: "Explica la memoria con color computado. Sin esto el agente no sabe qué es COGO.",
			Markdown: "## Memoria del proyecto (COGO)\n\n" +
				"Este proyecto usa **COGO** como memoria compartida entre agentes. No es un repositorio de archivos: " +
				"guarda **afirmaciones verificables** con un **color de confianza computado** (🟢 verde / 🟡 amarillo / 🔴 rojo).\n\n" +
				"El color lo calcula COGO a partir de la evidencia — **no lo decidís vos, lo obedecés**. " +
				"La ventaja: consumís un juicio ya hecho en vez de volver a derivarlo cada sesión.\n",
		},
		{
			ID: "repo-vs-cogo", Title: "Repo vs COGO", Essential: true,
			Desc: "La regla que evita el error más común: los archivos van al repo, el juicio a COGO.",
			Markdown: "### Dónde vive cada cosa\n\n" +
				"| | Dónde | Qué |\n|---|---|---|\n" +
				"| **Archivos** | el **repositorio** (git) | código, documentación, configuración |\n" +
				"| **Juicio** | **COGO** | decisiones, restricciones, bugs conocidos, runbooks, arquitectura |\n\n" +
				"**No metas archivos del proyecto en COGO.** El repo ya los versiona con historia y `diff`. " +
				"En COGO va lo que *no* está en los archivos o costaría re-derivar: por qué se decidió algo, " +
				"qué restricción rige, qué ya se probó y falló.\n",
		},
		{
			ID: "protocolo", Title: "Protocolo (obligatorio)", Essential: true,
			Desc: "Las 5 reglas: consultar antes de actuar, obedecer el color, capturar lo verificado.",
			Markdown: "### Protocolo (obligatorio)\n\n" +
				"1. **Consultá antes de actuar.** Antes de responder o cambiar algo, pedí contexto con `pack` " +
				"(o `search` para listar, `open` para una nota).\n" +
				"2. **Respetá el color.**\n" +
				"   - 🟢 **verde** = verificado. Podés apoyarte.\n" +
				"   - 🟡 **amarillo** = probable. Usalo con cautela y decí que es probable.\n" +
				"   - 🔴 **rojo** = **NO te apoyes**. Está en cuarentena: supuesto sin evidencia, cita rota o contradicción abierta.\n" +
				"3. **Capturá lo que verifiques** con `capture`: un *claim* declarativo + evidencia real (archivo, comando, log) " +
				"+ el `check` mínimo que lo probaría. **No escribas el color**: COGO lo computa.\n" +
				"4. **No pises el verde.** Si ya hay una nota verde, no la sobrescribas a ciegas: verificala de nuevo o usá un id nuevo.\n" +
				"5. **El rojo no se \"arregla\" escribiendo.** Una contradicción o una cita rota se resuelve corrigiendo la nota " +
				"o la evidencia, no cambiando el texto para que suene mejor.\n",
		},
		{
			ID: "conexion", Title: "Conexión (MCP)", Essential: true, Needs: "token",
			Desc: "El snippet .mcp.json para conectar este agente. Podés pegarle su token.",
			Markdown: "### Conexión (MCP)\n\n" + connSnippet(o) +
				"\n> Los tokens se emiten desde el visor (menú ⋮ → *Conexiones MCP*), **uno por agente**: " +
				"así la auditoría y el autor de cada nota dicen quién hizo qué.\n",
		},
		{
			ID: "proyecto", Title: "Proyecto", Needs: "project",
			Desc: "Ata al agente a un proyecto: pack/capture/recall filtrados, sin ruido de los otros.",
			Markdown: fmt.Sprintf("### Proyecto\n\nTrabajás sobre el proyecto **`%s`**. Usá SIEMPRE ese filtro:\n\n"+
				"- Contexto: `pack(query: \"<tema>\", project: \"%s\")`\n"+
				"- Al re-anclar (arranque o tras una compactación): `recall(project: \"%s\")` — trae el mandato y las decisiones verdes de este proyecto.\n"+
				"- Al guardar: `capture(project: \"%s\", …)`\n", proj, proj, proj, proj),
		},
		{
			ID: "repos-github", Title: "Repos por GitHub MCP",
			Desc: "Los archivos se leen del repo por el MCP oficial de GitHub; la evidencia se cita con github://.",
			Markdown: "### Los repositorios\n\n" +
				"El código se lee del **repositorio**, no de COGO. Si tenés conectado el **MCP oficial de GitHub** " +
				"(`github/github-mcp-server`), usalo para leer archivos, PRs e issues; COGO guarda el juicio sobre eso.\n\n" +
				"Cuando cites un archivo del repo como evidencia, **no uses una ruta local** (el COGO hosteado no tiene " +
				"tu working copy y la cita queda sin verificar). Citalo así:\n\n" +
				"```\ngithub://<owner>/<repo>@<rama-o-commit>/<ruta>:<línea>\n```\n\n" +
				"- COGO baja el archivo, confirma que la cita existe y guarda el **hash del blob**.\n" +
				"- Si citás una **rama** (`@main`), la nota se pone amarilla en cuanto ese archivo cambie: hay que re-verificar.\n" +
				"- Si citás un **commit fijo** (`@a1b2c3d`), la cita es inmutable y no puede driftear.\n",
		},
		{
			ID: "sincronizacion", Title: "Sincronización entre agentes",
			Desc: "Para ponerse al día con lo que hicieron otros agentes sin releer todo.",
			Markdown: "### Sincronización\n\n" +
				"El vault es compartido: otros agentes escriben en él. `recall` devuelve un **cursor** al final.\n\n" +
				"- Guardá ese cursor y en la próxima llamada usá `recall(since: \"<cursor>\")`: te da **solo lo que cambió** " +
				"desde entonces, en vez de releer toda la memoria.\n" +
				"- Si el mandato cambió, `recall` te avisa para que releas las líneas rojas.\n",
		},
		{
			ID: "leases", Title: "Coordinación (leases)",
			Desc: "Para que dos agentes no corran la misma migración o deploy.",
			Markdown: "### Coordinación con otros agentes\n\n" +
				"Antes de una tarea **no idempotente** (una migración, un deploy, una edición masiva), tomá un permiso:\n\n" +
				"- `lease(action: \"acquire\", name: \"<recurso>\", ttl_seconds: 900, note: \"<qué estás haciendo>\")`\n" +
				"- Si otro agente lo tiene, COGO te dice **quién** y **hasta cuándo** → **no arranques**, esperá o coordiná.\n" +
				"- Al terminar: `lease(action: \"release\", name: \"<recurso>\")`. Los leases expiran solos.\n\n" +
				"El permiso no es burocracia: es lo único que **frena** a otro agente. " +
				"Un `authorize` cuya acción nombre un permiso ajeno se **rechaza**, y ese rechazo no se destraba " +
				"verificando mejor — se destraba esperando o hablando. Si vos no tomás nada, nadie te va a frenar " +
				"y nadie te va a poder frenar.\n\n" +
				"Y si otro agente estuvo activo acá hace poco, `pack` y `authorize` te lo dicen al final de su " +
				"respuesta, sin que preguntes. Cuando aparezca ese bloque, leelo antes de escribir: " +
				"decir \"hay otro haciendo esto\" es un resultado válido de tu turno.\n",
		},
		{
			ID: "evidencia", Title: "Evidencia que se pierde (stash)",
			Desc: "Guardar por hash el log/CSV que prueba un claim, para que verify lo recompute.",
			Markdown: "### Evidencia que hoy se pierde\n\n" +
				"Cuando lo que prueba un claim es efímero (la salida completa de un comando que falló, un CSV, un archivo chico), " +
				"guardalo con `stash`: COGO lo almacena **por el hash de su contenido** y te devuelve `artifact://<sha>` para citarlo como evidencia.\n\n" +
				"- La referencia **prueba** que el artefacto no cambió, y `verify` lo recomputa en vez de confiar en una cita que se pudre.\n" +
				"- **No uses `stash` para documentación larga ni para código**: eso va al repositorio.\n" +
				"- Un guard de secretos corre antes de guardar y **rechaza** el contenido si detecta credenciales.\n",
		},
		{
			ID: "reflect", Title: "Cerrar con reflect",
			Desc: "Al terminar una tarea, que COGO proponga qué vale la pena capturar.",
			Markdown: "### Al cerrar una tarea\n\n" +
				"Pasale a `reflect` un resumen corto de lo que hiciste y verificaste. COGO te propone las notas que valen la pena " +
				"(claim + evidencia + check) para que no se pierda el hallazgo y no haya que re-derivarlo la próxima sesión. " +
				"Vos decidís cuáles capturar.\n",
		},
		{
			ID: "seguridad", Title: "Secretos y seguridad",
			Desc: "Nunca commitear credenciales; van por variables de entorno.",
			Markdown: "### Secretos\n\n" +
				"- **Nunca** escribas credenciales, claves de API ni tokens en el repositorio, en una nota de COGO ni en un artefacto.\n" +
				"- Las credenciales van por **variables de entorno** o el gestor de secretos del entorno.\n" +
				"- Si detectás un secreto filtrado, avisá y tratalo como comprometido (hay que rotarlo).\n",
		},
	}
}

func connSnippet(o BlockOptions) string {
	url := o.HTTPURL
	if url == "" {
		url = "https://TU-COGO/mcp"
	}
	tok := strings.TrimSpace(o.Token)
	if tok == "" {
		tok = "TU-TOKEN"
	}
	return "```json\n{\n  \"mcpServers\": {\n    \"cogo\": {\n      \"type\": \"http\",\n      \"url\": \"" +
		url + "\",\n      \"headers\": {\n        \"Authorization\": \"Bearer " + tok + "\"\n      }\n    }\n  }\n}\n```\n"
}

// Preset is a named set of blocks for a kind of agent — the 80% case in one click.
type Preset struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Desc   string   `json:"desc"`
	Blocks []string `json:"blocks"`
}

// Presets are the recommended compositions.
func Presets() []Preset {
	essentials := []string{"que-es-cogo", "repo-vs-cogo", "protocolo", "conexion"}
	with := func(extra ...string) []string { return append(append([]string{}, essentials...), extra...) }
	return []Preset{
		{ID: "minimo", Title: "Mínimo", Desc: "Solo lo esencial: qué es COGO, repo vs COGO, protocolo y conexión.", Blocks: essentials},
		{ID: "codigo", Title: "Agente de código", Desc: "Lo esencial + proyecto, repos por GitHub, evidencia, coordinación y cierre con reflect.", Blocks: with("proyecto", "repos-github", "evidencia", "leases", "reflect")},
		{ID: "research", Title: "Investigación", Desc: "Lo esencial + proyecto y sincronización con lo que hicieron otros agentes.", Blocks: with("proyecto", "sincronizacion", "reflect")},
		{ID: "ops", Title: "Operaciones", Desc: "Lo esencial + coordinación (leases), evidencia y secretos.", Blocks: with("leases", "evidencia", "seguridad")},
	}
}

// ---- custom blocks: the user's own reusable pieces ----

// customStore is the persisted list of user blocks, in the vault side-state.
type customStore struct {
	Blocks []Block `json:"blocks"`
}

var customMu sync.Mutex

func customPath(vault string) string { return filepath.Join(vault, ".cogo", "agent-blocks.json") }

// LoadCustom returns the user's own blocks (empty slice if none).
func LoadCustom(vault string) []Block {
	customMu.Lock()
	defer customMu.Unlock()
	var s customStore
	if b, err := os.ReadFile(customPath(vault)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	for i := range s.Blocks {
		s.Blocks[i].Custom = true
	}
	if s.Blocks == nil {
		return []Block{}
	}
	return s.Blocks
}

// SaveCustom adds or replaces a user block (matched by id) and persists it.
func SaveCustom(vault string, b Block) error {
	if strings.TrimSpace(b.Title) == "" || strings.TrimSpace(b.Markdown) == "" {
		return fmt.Errorf("un bloque necesita título y contenido")
	}
	b.Custom = true
	b.Essential = false
	if strings.TrimSpace(b.ID) == "" {
		b.ID = "mio-" + slug(b.Title)
	}
	customMu.Lock()
	defer customMu.Unlock()
	var s customStore
	if raw, err := os.ReadFile(customPath(vault)); err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	replaced := false
	for i := range s.Blocks {
		if s.Blocks[i].ID == b.ID {
			s.Blocks[i] = b
			replaced = true
			break
		}
	}
	if !replaced {
		s.Blocks = append(s.Blocks, b)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(customPath(vault)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(customPath(vault), raw, 0o644)
}

// DeleteCustom removes a user block by id.
func DeleteCustom(vault, id string) error {
	customMu.Lock()
	defer customMu.Unlock()
	var s customStore
	raw, err := os.ReadFile(customPath(vault))
	if err != nil {
		return nil // nothing stored: deleting is a no-op
	}
	_ = json.Unmarshal(raw, &s)
	out := s.Blocks[:0]
	for _, b := range s.Blocks {
		if b.ID != id {
			out = append(out, b)
		}
	}
	s.Blocks = out
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(customPath(vault), b, 0o644)
}

// slug makes a filename/id-safe token out of a title.
func slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
