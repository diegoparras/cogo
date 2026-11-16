package agentsmd

import (
	"strings"
	"testing"
)

func TestCuratedBlocks(t *testing.T) {
	bs := Curated(BlockOptions{HTTPURL: "https://cogo.example/mcp", Project: "talento", Token: "cogo_abc"})
	byID := map[string]Block{}
	for _, b := range bs {
		if b.ID == "" || b.Title == "" || strings.TrimSpace(b.Markdown) == "" {
			t.Errorf("block %+v is missing id/title/markdown", b)
		}
		if _, dup := byID[b.ID]; dup {
			t.Errorf("duplicate block id %q", b.ID)
		}
		byID[b.ID] = b
	}
	// The four must-haves are marked essential.
	for _, id := range []string{"que-es-cogo", "repo-vs-cogo", "protocolo", "conexion"} {
		if !byID[id].Essential {
			t.Errorf("%s should be essential", id)
		}
	}
	// Context actually lands in the text.
	if !strings.Contains(byID["conexion"].Markdown, "https://cogo.example/mcp") ||
		!strings.Contains(byID["conexion"].Markdown, "Bearer cogo_abc") {
		t.Errorf("connection block missing url/token:\n%s", byID["conexion"].Markdown)
	}
	if !strings.Contains(byID["proyecto"].Markdown, `project: "talento"`) {
		t.Errorf("project block missing the project name:\n%s", byID["proyecto"].Markdown)
	}
	// Without a token, a placeholder — never an empty Bearer.
	plain := Curated(BlockOptions{})
	for _, b := range plain {
		if b.ID == "conexion" && !strings.Contains(b.Markdown, "Bearer TU-TOKEN") {
			t.Errorf("no-token connection block should carry a placeholder:\n%s", b.Markdown)
		}
	}
}

func TestPresetsReferenceRealBlocks(t *testing.T) {
	known := map[string]bool{}
	for _, b := range Curated(BlockOptions{}) {
		known[b.ID] = true
	}
	for _, p := range Presets() {
		if len(p.Blocks) == 0 {
			t.Errorf("preset %s has no blocks", p.ID)
		}
		for _, id := range p.Blocks {
			if !known[id] {
				t.Errorf("preset %s references unknown block %q", p.ID, id)
			}
		}
	}
}

func TestCustomBlocksRoundTrip(t *testing.T) {
	vault := t.TempDir()
	if got := LoadCustom(vault); len(got) != 0 {
		t.Fatalf("fresh vault should have no custom blocks, got %v", got)
	}
	if err := SaveCustom(vault, Block{Title: "Convenciones del repo", Markdown: "usa tabs"}); err != nil {
		t.Fatal(err)
	}
	got := LoadCustom(vault)
	if len(got) != 1 || got[0].Title != "Convenciones del repo" || !got[0].Custom {
		t.Fatalf("custom block round-trip = %+v", got)
	}
	id := got[0].ID
	// Saving the same id replaces instead of duplicating.
	if err := SaveCustom(vault, Block{ID: id, Title: "Convenciones del repo", Markdown: "usa espacios"}); err != nil {
		t.Fatal(err)
	}
	if got = LoadCustom(vault); len(got) != 1 || got[0].Markdown != "usa espacios" {
		t.Fatalf("replace by id = %+v", got)
	}
	// Blocks need a title and content.
	if err := SaveCustom(vault, Block{Title: "  ", Markdown: "x"}); err == nil {
		t.Error("a block with no title should be rejected")
	}
	if err := DeleteCustom(vault, id); err != nil {
		t.Fatal(err)
	}
	if got = LoadCustom(vault); len(got) != 0 {
		t.Fatalf("after delete = %+v", got)
	}
}
