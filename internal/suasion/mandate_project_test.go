package suasion

import "testing"

func TestMandatePerProject(t *testing.T) {
	dir := t.TempDir()

	// Empty project resolves to the vault-wide path.
	if MandatePathFor(dir, "") != MandatePath(dir) {
		t.Fatal("empty project should map to the global mandate path")
	}
	// Project names are sanitized for the filename.
	if p := MandatePathFor(dir, "Talento ITP"); p == MandatePathFor(dir, "otro") {
		t.Fatal("different projects must map to different files")
	}

	if err := SaveMandate(MandatePath(dir), &Mandate{Goal: "global goal", RedLines: []string{"no global"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveMandate(MandatePathFor(dir, "talento"), &Mandate{RedLines: []string{"no talento thing"}}); err != nil {
		t.Fatal(err)
	}

	// A project with its own mandate uses it.
	if m := LoadMandateResolved(dir, "talento"); m == nil || len(m.RedLines) != 1 || m.RedLines[0] != "no talento thing" {
		t.Fatalf("talento mandate = %+v, want its own red line", m)
	}
	// A project without one falls back to the global mandate.
	if m := LoadMandateResolved(dir, "sinreglas"); m == nil || m.Goal != "global goal" {
		t.Fatalf("fallback mandate = %+v, want the global one", m)
	}
	// Empty project resolves straight to global.
	if m := LoadMandateResolved(dir, ""); m == nil || m.Goal != "global goal" {
		t.Fatalf("empty project = %+v, want global", m)
	}
}
