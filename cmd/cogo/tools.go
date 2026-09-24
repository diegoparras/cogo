package main

import (
	"context"
	"time"

	"github.com/diegoparras/cogo/internal/artifact"
	"github.com/diegoparras/cogo/internal/contra"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/ghsource"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// deps es lo que todos los tools necesitan del proceso: el vault, el caché,
// el escáner, el almacén de artefactos. Antes vivían como variables
// capturadas por dieciséis closures dentro de una función de seiscientas
// líneas; ahora cada tool está en su archivo y recibe esto.
type deps struct {
	dir            string
	scrubber       scrub.Scrubber
	store          artifact.Store
	cache          *core.VaultCache
	loadVault      func() (map[string]*core.Note, error)
	contradictions func() map[string]bool
}

// registradores es la lista de tools, uno por archivo. El orden es el de
// tools/list.
var registradores = []func(*mcp.Server, *deps){
	registrarPack,
	registrarStash,
	registrarRecall,
	registrarReflect,
	registrarLease,
	registrarSearch,
	registrarOpen,
	registrarGap,
	registrarAuthorize,
	registrarCapture,
	registrarVerify,
	registrarArchive,
	registrarRestore,
	registrarRemove,
	registrarGuard,
	registrarXray,
}

func newMCPServer(dir string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "cogo", Version: version}, nil)
	d := nuevasDeps(dir)
	for _, registrar := range registradores {
		registrar(s, d)
	}
	return s
}

// nuevasDeps arma lo que comparten los tools. Está separado de newMCPServer
// porque el hook de la CLI usa la misma política de `authorize` sin levantar
// un servidor.
func nuevasDeps(dir string) *deps {
	scrubber := scrub.FromEnv()
	store := artifact.FromEnv(dir) // R2 if COGO_R2_* is set, else disk under .cogo/artifacts
	// Resolve "artifact://<sha>" evidence against the store: present → the citation
	// holds; gone → it breaks and the color degrades. Content-addressed, so it can
	// only exist or not — verify recomputes instead of trusting a stale claim.
	core.SetArtifactChecker(func(sha string) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ok, _ := store.Has(ctx, sha)
		return ok
	})
	// Resolve "github://owner/repo@ref/path" evidence against the GitHub API, so a
	// hosted COGO (no working copy on disk) can still check file citations — and
	// so a note stays green only while the cited file's blob SHA hasn't moved.
	gh := ghsource.FromEnv()
	core.SetGitHubResolver(func(owner, repo, ref, path string) (string, bool, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		sha, found, err := gh.FileSHA(ctx, owner, repo, ref, path)
		if err != nil {
			return "", false, false // couldn't check: stays unchecked
		}
		return sha, found, true
	})
	cache := core.NewVaultCache(dir) // mtime-keyed reads: the MCP is a long-running server
	// loadVault reads the vault and checks that evidence refs resolve, so the
	// color an agent consumes reflects broken citations (see core.ResolveEvidence).
	// Evidence roots are re-read each call (tiny file) so UI edits take effect live.
	loadVault := func() (map[string]*core.Note, error) {
		v, err := cache.Load()
		if err != nil {
			return nil, err
		}
		core.ResolveEvidence(v, core.LoadEvidenceRoots(dir))
		return v, nil
	}
	// contradictions is the set of note ids under an OPEN contradiction, read fresh
	// from the persisted store each call (tiny file). Feeding it to the color engine
	// is what makes an agent over MCP see red-by-contradiction — the same paint the
	// visor shows — instead of a color blind to the store the human curates.
	contradictions := func() map[string]bool { return contra.Open(dir).OpenNoteSet() }

	return &deps{dir: dir, scrubber: scrubber, store: store, cache: cache,
		loadVault: loadVault, contradictions: contradictions}
}
