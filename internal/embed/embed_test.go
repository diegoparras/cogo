package embed

import (
	"context"
	"errors"
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
