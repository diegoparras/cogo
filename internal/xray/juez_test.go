package xray

import (
	"context"
	"strings"
	"testing"
)

type juezFalso struct{}

func (juezFalso) Radiografia(_ context.Context, claim string) (string, string, bool) {
	// "Sin ninguna duda" no está en la lista de boosters del léxico; el juez
	// lo lee igual. Y para la segunda frase se abstiene.
	if strings.HasPrefix(claim, "Sin ninguna duda") {
		return "boosted", "none", true
	}
	return "", "", false
}

func TestElJuezLeeLoQueElLexicoNoVe(t *testing.T) {
	texto := "Sin ninguna duda el pool aguanta doscientas conexiones. El cron corre de madrugada según el README."
	sin := Analyze(texto)
	con := AnalyzeCon(context.Background(), texto, juezFalso{})
	if len(sin.Claims) != 2 || len(con.Claims) != 2 {
		t.Fatalf("dos afirmaciones: %d %d", len(sin.Claims), len(con.Claims))
	}
	if sin.Claims[0].Color == "red" {
		t.Fatal("precondición: el léxico solo no la ve como afirmada con fuerza")
	}
	if con.Claims[0].Detector != "juez" || con.Claims[0].Commitment != "boosted" || con.Claims[0].Color != "red" {
		t.Fatalf("con el juez, fuerte sin fundamento es rojo: %+v", con.Claims[0])
	}
	// Donde el juez se abstiene, queda lo del léxico.
	if con.Claims[1].Detector != "" || con.Claims[1].Color != sin.Claims[1].Color {
		t.Fatalf("con abstención queda lo de siempre: %+v vs %+v", con.Claims[1], sin.Claims[1])
	}
}
