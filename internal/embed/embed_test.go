package embed

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEmbed returns a 3-dim bag-of-words vector over {redis,kafka,postgres} so
// cosine ranking is deterministic, and counts how many texts it was asked to embed.
type fakeEmbed struct{ texts int }

func (f *fakeEmbed) Embed(_ context.Context, ts []string) ([][]float32, error) {
	f.texts += len(ts)
	out := make([][]float32, len(ts))
	for i, t := range ts {
		l := strings.ToLower(t)
		out[i] = []float32{float32(strings.Count(l, "redis")), float32(strings.Count(l, "kafka")), float32(strings.Count(l, "postgres"))}
	}
	return out, nil
}

func TestRankAndCache(t *testing.T) {
	dir := t.TempDir()
	docs := []Doc{
		{ID: "a", Text: "the redis cache warms up"},
		{ID: "b", Text: "the kafka consumer lags"},
		{ID: "c", Text: "postgres is the store"},
	}
	fe := &fakeEmbed{}

	ids, err := Rank(context.Background(), dir, docs, "how does redis behave", fe)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 || ids[0] != "a" {
		t.Fatalf("redis query should rank note a first, got %v", ids)
	}
	if fe.texts != 4 { // 3 docs + 1 query
		t.Errorf("first run should embed 3 docs + query = 4, got %d", fe.texts)
	}

	// Second run, same docs: cache hit — only the query is re-embedded.
	fe.texts = 0
	ids2, _ := Rank(context.Background(), dir, docs, "kafka please", fe)
	if fe.texts != 1 {
		t.Errorf("cached run should embed only the query (1), got %d", fe.texts)
	}
	if ids2[0] != "b" {
		t.Errorf("kafka query should rank note b first, got %v", ids2)
	}

	// Change a doc's text → only that one re-embeds.
	docs[0].Text = "redis redis redis everywhere"
	fe.texts = 0
	_, _ = Rank(context.Background(), dir, docs, "x", fe)
	if fe.texts != 2 { // 1 changed doc + query
		t.Errorf("only the changed doc + query should embed (2), got %d", fe.texts)
	}
}

type errEmbed struct{}

func (errEmbed) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("boom")
}

func TestRankPropagatesError(t *testing.T) {
	if _, err := Rank(context.Background(), t.TempDir(), []Doc{{ID: "a", Text: "x"}}, "q", errEmbed{}); err == nil {
		t.Error("an embedder error must propagate so the caller can fall back")
	}
}

// TestFormatoBinario cubre el caché en disco: que un vector sobreviva el viaje a
// binario y vuelva idéntico, que el manifiesto quede LEGIBLE, y que las notas
// borradas no dejen basura.
func TestFormatoBinario(t *testing.T) {
	dir := t.TempDir()
	st := newStore()
	st.dims = 4
	st.vec["a"] = []float32{0.5, -1.25, 3, 0}
	st.hash["a"] = "h-a"
	st.vec["b"] = []float32{1, 1, 1, 1}
	st.hash["b"] = "h-b"
	if err := save(dir, st); err != nil {
		t.Fatal(err)
	}

	got := readFrom(dir)
	if got.dims != 4 || len(got.vec) != 2 {
		t.Fatalf("releído: dims=%d notas=%d", got.dims, len(got.vec))
	}
	for i, v := range got.vec["a"] {
		if v != st.vec["a"][i] {
			t.Errorf("el vector cambió al ir y volver: %v vs %v", got.vec["a"], st.vec["a"])
			break
		}
	}
	if got.hash["b"] != "h-b" {
		t.Errorf("se perdió el hash: %q", got.hash["b"])
	}

	// El manifiesto tiene que poder leerse e inspeccionarse: es la parte que un
	// humano querría abrir. El binario es opaco por naturaleza.
	mb, err := os.ReadFile(metaPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var m meta
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatalf("el manifiesto debería ser JSON legible: %v", err)
	}
	if m.Dims != 4 || len(m.Notes) != 2 || m.Notes["a"].Hash != "h-a" {
		t.Errorf("manifiesto incompleto: %+v", m)
	}
	// Y el binario pesa exactamente lo que dice: 2 vectores × 4 dims × 4 bytes.
	if fi, _ := os.Stat(binPath(dir)); fi == nil || fi.Size() != 32 {
		t.Errorf("el .bin debería pesar 32 bytes, pesa %v", fi)
	}

	if s := Describe(dir); s.Notes != 2 || s.Dims != 4 || s.Bytes != 32 {
		t.Errorf("Describe = %+v", s)
	}
}

// TestMigracionDesdeJSON: al actualizar, el caché viejo se convierte solo. Tirarlo
// obligaría a re-embeber todo, que cuesta llamadas al modelo — plata del usuario
// por una decisión nuestra.
func TestMigracionDesdeJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	viejo := `{"n1":{"h":"hh","v":[1,2,3]}}`
	if err := os.WriteFile(legacyPath(dir), []byte(viejo), 0o644); err != nil {
		t.Fatal(err)
	}
	st := readFrom(dir)
	if len(st.vec["n1"]) != 3 || st.hash["n1"] != "hh" {
		t.Fatalf("no migró: %+v", st.vec)
	}
	if _, err := os.Stat(metaPath(dir)); err != nil {
		t.Errorf("debería haber escrito el formato nuevo: %v", err)
	}
	if _, err := os.Stat(legacyPath(dir)); err == nil {
		t.Errorf("el JSON viejo debería borrarse tras migrar")
	}
}
