// Package agentsmd generates the bootstrap instruction file (AGENTS.md /
// CLAUDE.md) that a coding agent reads at the start of a session. COGO's context
// is pulled live over MCP, but the agent still needs to be TOLD the protocol —
// that a COGO memory exists, that the color is computed and must be obeyed, and
// how to connect. That on-ramp is what this file provides.
package agentsmd

import (
	"fmt"
	"strings"
)

// Options controls what the generated file says.
type Options struct {
	Filename string // "AGENTS.md" or "CLAUDE.md" — only changes the intro wording
	HTTPURL  string // MCP-over-HTTP endpoint, e.g. "http://localhost:8098/mcp"; wins over stdio
	Binary   string // path to the cogo binary, for the stdio .mcp.json snippet
	Vault    string // vault path, for the stdio args
	Digest   string // optional: a pre-rendered static snapshot to embed (see RenderDigest)
	Date     string // date the digest was taken, for the "regenerate" note
}

// Generate returns the full Markdown for the bootstrap file.
func Generate(o Options) string {
	name := o.Filename
	if name == "" {
		name = "AGENTS.md"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Memoria del proyecto (COGO)\n\n")
	b.WriteString("Este proyecto usa **COGO** como memoria: notas con un **color de confianza computado** ")
	b.WriteString("(verde/amarillo/rojo). El color ya viene calculado por COGO — no lo decidís vos, lo obedecés.\n\n")

	b.WriteString("## Protocolo (obligatorio)\n\n")
	b.WriteString("1. **Consultá antes de actuar.** Antes de responder o cambiar algo en este proyecto, ")
	b.WriteString("pedí contexto a COGO con la herramienta MCP `pack` (o `search` para listar, `open` para una nota). ")
	b.WriteString("Trae lo que ya se sabe, coloreado.\n")
	b.WriteString("2. **Respetá el color.**\n")
	b.WriteString("   - 🟢 **verde** = verificado. Podés apoyarte.\n")
	b.WriteString("   - 🟡 **amarillo** = probable. Usalo con cautela y decí que es probable.\n")
	b.WriteString("   - 🔴 **rojo** = **NO te apoyes**. Está en cuarentena (`pack` ya lo degrada a *do-not-rely*): ")
	b.WriteString("es un supuesto sin evidencia, una cita rota, o una contradicción abierta. Tratalo como \"esto puede estar mal\".\n")
	b.WriteString("3. **Capturá lo que verifiques.** Cuando confirmes algo nuevo, guardalo con `capture`: ")
	b.WriteString("un *claim* declarativo + la evidencia (archivo, comando, log) + el `check` que lo probaría. ")
	b.WriteString("No escribas el color: COGO lo computa a partir de la evidencia.\n")
	b.WriteString("4. **No pises el verde.** Si ya existe una nota verde, no la sobrescribas a ciegas: verificala de nuevo o usá un id nuevo.\n")
	b.WriteString("5. **El rojo no se \"arregla\" escribiendo.** Una contradicción o una cita rota se resuelve corrigiendo la nota o la evidencia, no cambiando el texto para que suene mejor.\n\n")

	b.WriteString("## Conexión (MCP)\n\n")
	b.WriteString(connectionSnippet(o))
	b.WriteString("\n")

	if strings.TrimSpace(o.Digest) != "" {
		b.WriteString("---\n\n")
		fmt.Fprintf(&b, "## Instantánea de la memoria")
		if o.Date != "" {
			fmt.Fprintf(&b, " (al %s)", o.Date)
		}
		b.WriteString("\n\n")
		b.WriteString("> Snapshot estático para un agente que **no** habla MCP. Se queda viejo: ")
		b.WriteString("la fuente de verdad es COGO en vivo. Regeneralo con `cogo agents --digest`.\n\n")
		b.WriteString(strings.TrimRight(o.Digest, "\n"))
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "\n<!-- Generado por COGO para %s. Regeneralo cuando cambie el protocolo o la conexión. -->\n", name)
	return b.String()
}

func connectionSnippet(o Options) string {
	if o.HTTPURL != "" {
		return "Este agente ya puede hablar con COGO por HTTP. Config para el `.mcp.json` del cliente:\n\n" +
			"```json\n{\n  \"mcpServers\": {\n    \"cogo\": {\n      \"type\": \"http\",\n      \"url\": \"" + o.HTTPURL + "\"\n    }\n  }\n}\n```\n"
	}
	bin := o.Binary
	if bin == "" {
		bin = "cogo"
	}
	vault := o.Vault
	if vault == "" {
		vault = "."
	}
	return "Levantá COGO por stdio desde el cliente. Config para el `.mcp.json`:\n\n" +
		"```json\n{\n  \"mcpServers\": {\n    \"cogo\": {\n      \"command\": \"" + jsonEscape(bin) + "\",\n      \"args\": [\"serve\", \"-vault\", \"" + jsonEscape(vault) + "\"]\n    }\n  }\n}\n```\n"
}

func jsonEscape(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}

// DigestItem is one note distilled for the static snapshot.
type DigestItem struct{ Color, ID, Claim string }

// RenderDigest lists the green and yellow notes as a compact, human-readable
// snapshot. Red/ungraded are skipped on purpose: the snapshot is "what you can
// lean on", and red is do-not-rely — no point freezing it into a static file.
func RenderDigest(items []DigestItem) string {
	var g, y strings.Builder
	ng, ny := 0, 0
	for _, it := range items {
		line := fmt.Sprintf("- `%s` — %s\n", it.ID, oneLine(it.Claim))
		switch it.Color {
		case "green":
			g.WriteString(line)
			ng++
		case "yellow":
			y.WriteString(line)
			ny++
		}
	}
	if ng == 0 && ny == 0 {
		return "_(sin notas verdes ni amarillas todavía)_\n"
	}
	var b strings.Builder
	if ng > 0 {
		b.WriteString("### 🟢 Verificado\n\n")
		b.WriteString(g.String())
		b.WriteString("\n")
	}
	if ny > 0 {
		b.WriteString("### 🟡 Probable\n\n")
		b.WriteString(y.String())
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "**Claim:**"))
	if r := []rune(s); len(r) > 140 {
		s = string(r[:139]) + "…"
	}
	return s
}
