package agentdocs

import "testing"

func TestSafeName(t *testing.T) {
	ok := []string{"AGENTS.md", "CLAUDE.md", "copilot-instructions.md", "my_notes.md"}
	for _, n := range ok {
		if SafeName(n) != n {
			t.Errorf("%q should be a safe name", n)
		}
	}
	bad := []string{"", "../etc/passwd", ".hidden.md", "no-ext", "a/b.md", "..md", "x.txt"}
	for _, n := range bad {
		if SafeName(n) != "" {
			t.Errorf("%q must be rejected, got %q", n, SafeName(n))
		}
	}
}

func TestSaveLoadHistory(t *testing.T) {
	dir := t.TempDir()

	if c, _ := Load(dir, "AGENTS.md"); c != "" {
		t.Errorf("missing doc should load as empty, got %q", c)
	}
	if err := Save(dir, "AGENTS.md", "v1", "2026-07-04T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, "AGENTS.md", "v2", "2026-07-05T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(dir, "AGENTS.md"); c != "v2" {
		t.Errorf("current content should be latest, got %q", c)
	}
	h := History(dir, "AGENTS.md")
	if len(h) != 2 || h[0].Content != "v1" || h[1].Content != "v2" {
		t.Errorf("history should keep both revisions oldest-first, got %+v", h)
	}

	// List sees the doc; a bad name never writes.
	if names := List(dir); len(names) != 1 || names[0].Name != "AGENTS.md" {
		t.Errorf("List should return the saved doc, got %+v", names)
	}
	if err := Save(dir, "../evil", "x", "t"); err == nil {
		t.Error("saving an unsafe name must error")
	}

	// Delete removes doc + history.
	if err := Delete(dir, "AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(dir, "AGENTS.md"); c != "" {
		t.Error("deleted doc should be gone")
	}
	if len(History(dir, "AGENTS.md")) != 0 {
		t.Error("history should be gone after delete")
	}
}
