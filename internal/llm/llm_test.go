package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleComplete(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-Title") != "COGO" {
			t.Errorf("missing X-Title attribution header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "YES: A says up, B says down"}},
			},
		})
	}))
	defer ts.Close()

	p := &OpenAICompatible{BaseURL: ts.URL, Model: "test-model"}
	if !p.Available() {
		t.Fatal("provider should be available")
	}
	out, err := p.Complete(context.Background(), "compare")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "YES") {
		t.Errorf("got %q", out)
	}
}

func TestFromEnvOffByDefault(t *testing.T) {
	t.Setenv("COGO_LLM_BASE_URL", "")
	t.Setenv("COGO_LLM_MODEL", "")
	if FromEnv().Available() {
		t.Error("with no env, the provider must be off (Noop)")
	}
}
