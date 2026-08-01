package ghsource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileSHA(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case strings.Contains(r.URL.Path, "existe.go"):
			if got := r.URL.Query().Get("ref"); got != "main" {
				t.Errorf("ref = %q, want main", got)
			}
			fmt.Fprint(w, `{"sha":"abc123","type":"file"}`)
		case strings.Contains(r.URL.Path, "carpeta"):
			fmt.Fprint(w, `[{"name":"x"}]`) // un directorio no es una cita
		case strings.Contains(r.URL.Path, "limite"):
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := FromEnv()
	c.api = srv.URL

	ctx := context.Background()
	sha, found, err := c.FileSHA(ctx, "o", "r", "main", "dir/existe.go")
	if err != nil || !found || sha != "abc123" {
		t.Fatalf("archivo existente = (%q,%v,%v)", sha, found, err)
	}
	// El caché evita golpear la API una vez por nota por request.
	before := calls
	if _, _, err := c.FileSHA(ctx, "o", "r", "main", "dir/existe.go"); err != nil {
		t.Fatal(err)
	}
	if calls != before {
		t.Errorf("la segunda llamada no usó el caché (%d -> %d)", before, calls)
	}

	if _, found, err := c.FileSHA(ctx, "o", "r", "main", "falta.go"); err != nil || found {
		t.Errorf("404 debe ser found=false sin error, got (%v,%v)", found, err)
	}
	if _, found, err := c.FileSHA(ctx, "o", "r", "main", "carpeta"); err != nil || found {
		t.Errorf("un directorio no es un archivo citable, got (%v,%v)", found, err)
	}
	// 403 (rate limit / sin acceso) DEBE ser error, para que el color quede
	// "unchecked" en vez de castigar la nota como si la cita estuviera rota.
	if _, _, err := c.FileSHA(ctx, "o", "r", "main", "limite.go"); err == nil {
		t.Error("403 debería devolver error (unchecked), no found=false (broken)")
	}
}

func TestEscapePath(t *testing.T) {
	if got := escapePath("/a b/c.go"); got != "a%20b/c.go" {
		t.Errorf("escapePath = %q", got)
	}
}
