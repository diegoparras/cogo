// Package core holds every COGO rule: parse, color, pack, graph, lint.
// It is written once; every face (CLI, MCP server, web) goes through it, and
// nothing else ever touches the vault. The color is computed here, never
// hand-written, so any agent consumes the result without knowing the rules.
package core

// Color is the computed confidence semaphore. It is the identity of COGO and is
// never supplied by a human or an agent — only derived (see Evaluate).
type Color int

const (
	Ungraded Color = iota // mistakes: informational, not graded by confidence
	Green                 // verified
	Yellow                // probable
	Red                   // unverified / assumption — "do not rely on this"
)

func (c Color) String() string {
	switch c {
	case Green:
		return "green"
	case Yellow:
		return "yellow"
	case Red:
		return "red"
	default:
		return "ungraded"
	}
}

// Evidence is one supporting artifact. The strongest item sets the note's tier
// (see §4). An item with an empty Ref carries no weight (treated as none),
// because evidence without a reference to a real artifact is just a claim.
type Evidence struct {
	Kind string `yaml:"kind" json:"kind"`
	Ref  string `yaml:"ref" json:"ref"`
	// Hash is a content hash of the cited file at the moment the note was last
	// verified (persisted). It is the drift baseline: if the file changes after
	// verification, the note can no longer be green — the evidence moved under it.
	// Empty for non-file evidence or notes never verified through COGO.
	Hash string `yaml:"hash,omitempty" json:"-"`
	// Anchor es la huella de la REGIÓN que la cita señala, y AnchorAt el tramo de
	// líneas del que se tomó. Juntos son lo que permite distinguir un archivo que
	// cambió de una cita que cambió: sin ellos, tocar la línea 3 invalida una nota
	// que citaba la 164. Ver ancla.go.
	Anchor   string `yaml:"anchor,omitempty" json:"-"`
	AnchorAt string `yaml:"anchor_at,omitempty" json:"-"`
	// Status is computed at runtime by ResolveEvidence (not persisted): whether
	// the ref actually points at something real. A "broken" item stops counting
	// toward the note's color — that is the difference between an honest green
	// and a claimed one. "drifted" = the file changed WHERE the note cited;
	// "moved" = it changed elsewhere, or the citation just shifted lines.
	Status string `yaml:"-" json:"status,omitempty"` // resolved | broken | unchecked | drifted | moved
	// Detail explica el status en una frase, cuando hay algo que explicar. No se
	// persiste: se recalcula, como el color.
	Detail string `yaml:"-" json:"detail,omitempty"`
}

// Check is the minimal test that would verify the claim.
type Check struct {
	Test   string `yaml:"test"`
	Status string `yaml:"status"` // passed | failed | not_run

	// Attested dice CÓMO se estableció ese status, que es un eje distinto del
	// color y no debe confundirse con él:
	//
	//	declared — alguien afirmó que el check pasa. Nadie lo corrió.
	//	executed — COGO ejecutó el comando y vio su código de salida.
	//
	// El color mide cuánto respalda la evidencia; esto mide quién lo comprobó.
	// Separarlos es lo que permite que una nota siga siendo verde sin que el
	// verde mienta: dice "la evidencia sostiene esto", y el sello dice si se
	// ejecutó o se declaró. Vacío en notas viejas = declared.
	Attested   string `yaml:"attested,omitempty"`
	AttestedBy string `yaml:"attested_by,omitempty"` // quién lo afirmó o lo corrió
}

// Los dos valores de Check.Attested.
const (
	AttestDeclared = "declared"
	AttestExecuted = "executed"
)

// Attestation devuelve la procedencia del check, tratando el campo vacío de las
// notas anteriores a este campo como una declaración — que es lo que eran.
func (c Check) Attestation() string {
	if c.Attested == AttestExecuted {
		return AttestExecuted
	}
	return AttestDeclared
}

// Note is one Markdown note in the vault. The fields above the line are inputs,
// supplied by a human or an agent. The fields below (Confidence, StaleAt,
// ColorReason) are computed by COGO and must not be hand-edited.
type Note struct {
	ID           string     `yaml:"id"`   // stable, independent of filename
	Type         string     `yaml:"type"` // decision|bug|runbook|architecture|constraint|command|mistake
	Project      string     `yaml:"project"`
	Evidence     []Evidence `yaml:"evidence"`
	Check        Check      `yaml:"check"`
	LastVerified Date       `yaml:"last_verified"`
	DependsOn    []string   `yaml:"depends_on"` // hard graph edges this note rests on
	Supersedes   string     `yaml:"supersedes"`
	CausedBy     string     `yaml:"caused_by"`
	Status       string     `yaml:"status"` // "" (active) | archived | retracted — the lifecycle axis, orthogonal to color

	// Author is who captured the note (the authenticated caller: "root",
	// "user:<email>", "token:<label>"). On a vault shared across machines it says
	// which agent wrote the claim.
	Author string `yaml:"author,omitempty"`
	// Scope records the conditions under which the claim held — os, arch, commit,
	// runtime versions (e.g. {os: windows, go: "1.25"}). A note true on Windows can
	// be false on Linux; recording the scope keeps it from being trusted blindly on
	// another machine. Free-form keys; ScopeConflict compares against an env.
	Scope map[string]string `yaml:"scope,omitempty"`

	// El eje de ORIGEN: quién originó la afirmación, distinto de quién escribió
	// la nota (Author) y de quién atestiguó el check (Check.AttestedBy). Ver
	// origen.go: es lo que impide que un agente lave su propia propuesta en una
	// decisión del proyecto.
	Origin string `yaml:"origin,omitempty" json:"origin,omitempty"`
	// Pinned saca a la nota del olvido, para siempre. Es la salida de emergencia
	// del que sabe algo que las reglas no: "esto no se toca aunque parezca
	// muerto". Ver latencia.go — no afecta al color, solo a si sale del camino.
	Pinned bool `yaml:"pinned,omitempty" json:"pinned,omitempty"`

	// ---- notas de brecha (type: gap) ----
	//
	// Una brecha no es una nota sin evidencia: es una PREGUNTA ABIERTA. La
	// diferencia importa. Una nota roja afirma algo sin respaldo; una brecha no
	// afirma nada — declara que hay algo que el proyecto no sabe, y que hay
	// decisiones esperando esa respuesta.
	//
	// Sin esto, un agente no puede distinguir un tema que nadie investigó de un
	// tema que no existe. Las dos ausencias se ven igual: silencio.

	// Question es lo que no se sabe, escrito como pregunta. Es el campo central:
	// una brecha bien planteada es media brecha resuelta.
	Question string `yaml:"question,omitempty"`
	// Blocks son las decisiones que están esperando esta respuesta. El conteo
	// ordena la lista: conviene resolver primero lo que traba más cosas.
	Blocks []string `yaml:"blocks,omitempty"`
	// CostToResolve estima cuánto cuesta averiguarlo: bajo | medio | alto.
	// Junto con cuántas decisiones traba, es lo que permite elegir por dónde.
	CostToResolve string `yaml:"cost_to_resolve,omitempty"`
	// Attempted registra lo que ya se probó y por qué no alcanzó. Es lo que
	// evita que tres personas distintas choquen contra la misma pared.
	Attempted []string `yaml:"attempted,omitempty"`

	// ---- computed by COGO · do not edit ----
	Confidence  string `yaml:"confidence"`
	StaleAt     Date   `yaml:"stale_at"`
	ColorReason string `yaml:"color_reason"`

	Body string `yaml:"-"` // markdown after the frontmatter
	Path string `yaml:"-"` // file the note was read from (set by ReadNoteFile)
}

// Apply writes a computed Verdict back onto the note's computed fields.
func (n *Note) Apply(v Verdict) {
	n.Confidence = v.Color.String()
	n.StaleAt = v.StaleAt
	n.ColorReason = v.Reason
}
