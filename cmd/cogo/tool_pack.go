package main

import (
	"context"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/savings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarPack registra el tool `pack`. Ver tools.go para lo que comparten.
func registrarPack(s *mcp.Server, d *deps) {
	dir, cache, loadVault, contradictions := d.dir, d.cache, d.loadVault, d.contradictions
	mcp.AddTool(s, &mcp.Tool{
		Name:        "pack",
		Description: "Get colored context for a topic before acting. Green=verified, yellow=probable; red is quarantined as do-not-rely.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in packIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		p := core.BuildPack(vault, contradictions(), core.PackOptions{Query: in.Query, Project: in.Project, Budget: in.Budget, Today: today(), Env: in.Env})
		// Lo que entró en el pack se consumió. Es la señal que permite olvidar sin
		// riesgo: una nota que ningún pack entrega hace medio año no la extraña
		// nadie. Y es lo que DESPIERTA a una latente, porque la latencia se calcula
		// —no se escribe— así que dejar de estar sin consultar alcanza.
		Consultadas(p.Incluidas...)
		// Y le cuelga quién más está trabajando acá. COGO no puede empujar, pero
		// el agente ya está obligado a pedir contexto antes de actuar: esta
		// respuesta es el canal, y el aviso llega justo cuando sirve.
		aviso := avisoDeOtros(ctx, dir, in.Project) + avisoDeProblemas(cache.Problemas())
		savings.Add(dir, p.RawTokens-p.Tokens, today().String())
		return textResult(p.Markdown + aviso), nil, nil
	})
}
