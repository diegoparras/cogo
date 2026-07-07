// Package embed is the OPTIONAL semantic-search accessory. It ranks notes by
// meaning (embedding cosine) instead of keywords — closing the "keyword search
// misses relevant memory" gap. Like the whole model layer it is OFF unless an
// embedding model is configured; core stays deterministic and never depends on it.
//
// Embeddings are cached in the vault (.cogo/embeddings.json) keyed by the content
// hash of each note's text, so only changed/new notes are re-embedded — one HTTP
// batch call per search at most.
package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// Embedder is the minimal contract (satisfied by llm.OpenAICompatible).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Doc is one note reduced to what gets embedded: its id and a short text (claim).
type Doc struct{ ID, Text string }

type entry struct {
	Hash string    `json:"h"`
	Vec  []float32 `json:"v"`
}

func cachePath(dir string) string { return filepath.Join(dir, ".cogo", "embeddings.json") }

func hashText(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%016x", h.Sum64())
}

// Rank returns the doc ids ordered by semantic similarity to query (most similar
// first). It (re)embeds only docs whose text changed since last time, batches the
// misses in one call, persists the cache, and prunes vanished notes. Returns an
// error if the embedder fails — the caller falls back to keyword ranking.
func Rank(ctx context.Context, dir string, docs []Doc, query string, e Embedder) ([]string, error) {
	cache := map[string]entry{}
	if b, err := os.ReadFile(cachePath(dir)); err == nil {
		_ = json.Unmarshal(b, &cache)
	}

	var missIdx []int
	var missText []string
	present := map[string]bool{}
	for i, d := range docs {
		present[d.ID] = true
		h := hashText(d.Text)
		if c, ok := cache[d.ID]; !ok || c.Hash != h || len(c.Vec) == 0 {
			missIdx = append(missIdx, i)
			missText = append(missText, d.Text)
		}
	}
	if len(missText) > 0 {
		vecs, err := e.Embed(ctx, missText)
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(missText) {
			return nil, fmt.Errorf("embed: got %d vectors for %d texts", len(vecs), len(missText))
		}
		for j, idx := range missIdx {
			cache[docs[idx].ID] = entry{Hash: hashText(docs[idx].Text), Vec: vecs[j]}
		}
	}
	for id := range cache {
		if !present[id] {
			delete(cache, id) // note gone from the vault
		}
	}
	if b, err := json.Marshal(cache); err == nil {
		_ = os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755)
		_ = os.WriteFile(cachePath(dir), b, 0o644)
	}

	qv, err := e.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(qv) == 0 || len(qv[0]) == 0 {
		return nil, fmt.Errorf("embed: empty query vector")
	}

	type scored struct {
		id string
		s  float64
	}
	ranked := make([]scored, 0, len(docs))
	for _, d := range docs {
		ranked = append(ranked, scored{d.ID, cosine(qv[0], cache[d.ID].Vec)})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].s != ranked[j].s {
			return ranked[i].s > ranked[j].s
		}
		return ranked[i].id < ranked[j].id
	})
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.id
	}
	return out, nil
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
