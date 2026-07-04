package agentsmd

import (
	"strings"
	"testing"
)

func TestGenerateHTTP(t *testing.T) {
	md := Generate(Options{Filename: "AGENTS.md", HTTPURL: "http://localhost:8098/mcp"})
	for _, want := range []string{"COGO", "Protocolo", "pack", "🔴", "http://localhost:8098/mcp", "\"type\": \"http\""} {
		if !strings.Contains(md, want) {
			t.Errorf("http output missing %q", want)
		}
	}
	if strings.Contains(md, "Instantánea") {
		t.Error("no digest requested but snapshot present")
	}
}

func TestGenerateStdioAndDigest(t *testing.T) {
	md := Generate(Options{
		Filename: "CLAUDE.md",
		Binary:   `C:\bin\cogo.exe`,
		Vault:    `C:\vault`,
		Digest:   RenderDigest([]DigestItem{{Color: "green", ID: "a", Claim: "**Claim:** X funciona"}, {Color: "yellow", ID: "b", Claim: "Y probable"}, {Color: "red", ID: "c", Claim: "Z sin evidencia"}}),
		Date:     "2026-07-04",
	})
	// stdio snippet: backslashes must be JSON-escaped.
	if !strings.Contains(md, `C:\\bin\\cogo.exe`) {
		t.Error("stdio binary path not JSON-escaped")
	}
	if !strings.Contains(md, `"serve", "-vault"`) {
		t.Error("stdio args missing")
	}
	// digest groups green and yellow, drops red (do-not-rely, no point freezing).
	if !strings.Contains(md, "🟢 Verificado") || !strings.Contains(md, "`a`") {
		t.Error("digest missing green section")
	}
	if !strings.Contains(md, "🟡 Probable") || !strings.Contains(md, "`b`") {
		t.Error("digest missing yellow section")
	}
	if strings.Contains(md, "`c`") {
		t.Error("red note must not appear in the static snapshot")
	}
}

func TestRenderDigestEmpty(t *testing.T) {
	if got := RenderDigest(nil); !strings.Contains(got, "sin notas") {
		t.Errorf("empty digest should say so, got %q", got)
	}
}
