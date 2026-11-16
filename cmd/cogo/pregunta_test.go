package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/decision"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// conectarConPersona arma un cliente que, si COGO pregunta, contesta lo que
// diga `responde` y guarda el mensaje que vio la persona.
func conectarConPersona(t *testing.T, ctx context.Context, dir, responde, protocolo string, mensaje *string) *mcp.ClientSession {
	t.Helper()
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(dir).Run(ctx, serverT) }()
	var opts *mcp.ClientOptions
	if responde != "" {
		opts = &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			*mensaje = r.Params.Message
			return &mcp.ElicitResult{Action: responde}, nil
		}}
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, opts).Connect(ctx, clientT, &mcp.ClientSessionOptions{ProtocolVersion: protocolo})
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

// Cuando el que pide es el agente y el cliente sabe preguntar, lo que COGO no
// autoriza se lo pregunta a la persona por MCP, y la respuesta queda asentada.
func TestAuthorizeLePreguntaALaPersona(t *testing.T) {
	casos := []struct {
		nombre, responde, protocolo string
		autoriza                    bool
		estado                      decision.Estado
	}{
		{"acepta", "accept", "", true, decision.Aprobada},
		{"rechaza", "decline", "", false, decision.Rechazada},
		{"cierra sin contestar", "cancel", "", false, decision.Rechazada},
		{"cliente de la spec anterior", "accept", "2025-11-25", true, decision.Aprobada},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			soltarRegistro()
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var mensaje string
			cs := conectarConPersona(t, ctx, dir, c.responde, c.protocolo, &mensaje)
			defer cs.Close()

			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "authorize", Arguments: map[string]any{
				"action": "borrar la base de staging", "find_support": true,
			}})
			if err != nil {
				t.Fatal(err)
			}
			txt := toolText(res)
			if !strings.Contains(mensaje, "borrar la base de staging") || !strings.Contains(mensaje, "r-") {
				t.Fatalf("la persona tiene que ver la acción y el recibo: %q", mensaje)
			}
			if c.autoriza && !strings.HasPrefix(txt, "AUTHORIZED BY THE PERSON") {
				t.Fatalf("aceptó: %s", txt)
			}
			if !c.autoriza && !strings.HasPrefix(txt, "NOT AUTHORIZED") {
				t.Fatalf("no aceptó: %s", txt)
			}
			var vistas []decision.Decision
			for _, d := range decision.Abrir(dir).Estados(time.Now().UTC()) {
				vistas = append(vistas, d)
			}
			if len(vistas) != 1 || vistas[0].Estado != c.estado || vistas[0].Via != decision.ViaPregunta {
				t.Fatalf("la respuesta tiene que quedar asentada como %s por pregunta: %+v", c.estado, vistas)
			}
		})
	}
}

// La respuesta la trae el cliente. Si nombra un recibo que no es una pregunta
// abierta sobre esta misma acción, no aprueba nada.
func TestAuthorizeNoAceptaUnaRespuestaInventada(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var mensaje string
	cs := conectarConPersona(t, ctx, dir, "decline", "", &mensaje)
	defer cs.Close()
	// Una pregunta real, que la persona rechaza, deja un recibo conocido.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "authorize", Arguments: map[string]any{"action": "borrar la base de staging", "find_support": true}}); err != nil {
		t.Fatal(err)
	}
	var real string
	for id := range decision.Abrir(dir).Estados(time.Now().UTC()) {
		real = id
	}
	for _, estado := range []string{"r-20990101-000000-abcdef", real} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name:           "authorize",
			Arguments:      map[string]any{"action": "borrar la base de staging"},
			InputResponses: mcp.InputResponseMap{clavePregunta: &mcp.ElicitResult{Action: "accept"}},
			RequestState:   estado,
		})
		if err == nil && !res.IsError {
			t.Fatalf("una respuesta a %q que no es una pregunta abierta no puede aprobar: %s", estado, toolText(res))
		}
	}
	for _, d := range decision.Abrir(dir).Estados(time.Now().UTC()) {
		if d.Estado == decision.Aprobada {
			t.Fatalf("no tenía que quedar nada aprobado: %+v", d)
		}
	}
}

// Sin un cliente que sepa preguntar, todo sigue como antes: NOT AUTHORIZED, y
// no queda ninguna decisión abierta.
func TestAuthorizeSinClienteQuePregunte(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cs := conectarConPersona(t, ctx, dir, "", "", nil)
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "authorize", Arguments: map[string]any{
		"action": "borrar la base de staging", "find_support": true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := toolText(res); !strings.HasPrefix(txt, "NOT AUTHORIZED") {
		t.Fatalf("sin a quién preguntar, se frena: %s", txt)
	}
	if n := len(decision.Abrir(dir).Estados(time.Now().UTC())); n != 0 {
		t.Fatalf("sin pregunta no hay decisión: %d", n)
	}
}
