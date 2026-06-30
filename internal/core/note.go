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
}

// Check is the minimal test that would verify the claim.
type Check struct {
	Test   string `yaml:"test"`
	Status string `yaml:"status"` // passed | failed | not_run
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
