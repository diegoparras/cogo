package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// La CLI hablando con un COGO hosteado.
//
// Hasta acá la CLI solo abría un directorio. Pero el vault que importa vive en
// el servidor, y el hook corre en la máquina del agente: si no puede hablar con
// el hosteado, el hook consulta un vault que no es el que todos escriben.
//
// Es un cliente MCP común, por Streamable HTTP, con el Bearer que el servidor
// ya entiende. No hay protocolo nuevo: la CLI usa los mismos 16 tools que
// cualquier agente.

// conBearer agrega el token a cada request. Es un RoundTripper y no un header
// puesto a mano porque el transporte del SDK hace varios requests por sesión.
type conBearer struct {
	token string
	base  http.RoundTripper
}

func (c conBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	if c.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.base.RoundTrip(r)
}

// conectarRemoto abre una sesión MCP contra `url`. Sin el stream SSE aparte:
// la CLI pregunta y se va, no espera notificaciones.
func conectarRemoto(ctx context.Context, url, token string) (*mcp.ClientSession, error) {
	t := &mcp.StreamableClientTransport{
		Endpoint:             url,
		DisableStandaloneSSE: true,
		MaxRetries:           1,
		HTTPClient: &http.Client{
			Timeout:   20 * time.Second,
			Transport: conBearer{token: token, base: http.DefaultTransport},
		},
	}
	cli := mcp.NewClient(&mcp.Implementation{Name: "cogo-cli", Version: version}, nil)
	cs, err := cli.Connect(ctx, t, nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a %s: %w", url, err)
	}
	return cs, nil
}

// llamarRemoto invoca un tool y devuelve su texto y su salida estructurada.
func llamarRemoto(ctx context.Context, cs *mcp.ClientSession, tool string, args map[string]any) (string, map[string]any, error) {
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return "", nil, err
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if res.IsError {
		return b.String(), nil, fmt.Errorf("%s", strings.TrimPrefix(b.String(), "cogo: "))
	}
	var est map[string]any
	if res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			_ = json.Unmarshal(raw, &est)
		}
	}
	return b.String(), est, nil
}
