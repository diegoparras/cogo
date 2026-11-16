package decision

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)

func TestAprobarUnPedido(t *testing.T) {
	s := Abrir(t.TempDir())
	if err := s.Pedir(Pedido{Recibo: "r-1", ToolUseID: "toolu_1", Accion: "rm -rf build", Via: ViaHook, Cuando: t0}); err != nil {
		t.Fatal(err)
	}
	if r, ok := s.PorToolUse("toolu_1"); !ok || r != "r-1" {
		t.Fatalf("el pedido se busca por tool_use_id: %q %v", r, ok)
	}
	if err := s.Resolver("r-1", Aprobada, "user:diego", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	d := s.Estados(t0.Add(time.Hour))["r-1"]
	if d.Estado != Aprobada || d.Quien != "user:diego" || d.Accion != "rm -rf build" {
		t.Fatalf("aprobada: %+v", d)
	}
}

// Un pedido sin respuesta vence, y una vencida no vuelve a aprobada aunque la
// aprobación llegue tarde: el plan lo pone como invariante.
func TestUnPendienteVenceYNoRevive(t *testing.T) {
	s := Abrir(t.TempDir())
	_ = s.Pedir(Pedido{Recibo: "r-1", Accion: "drop table", Via: ViaHook, Cuando: t0})
	if d := s.Estados(t0.Add(Vence - time.Second))["r-1"]; d.Estado != Pendiente {
		t.Fatalf("antes de vencer sigue pendiente: %+v", d)
	}
	if d := s.Estados(t0.Add(Vence + time.Second))["r-1"]; d.Estado != Vencida {
		t.Fatalf("sin respuesta vence: %+v", d)
	}
	_ = s.Resolver("r-1", Aprobada, "user:diego", t0.Add(Vence+time.Minute))
	if d := s.Estados(t0.Add(2 * Vence))["r-1"]; d.Estado != Vencida {
		t.Fatalf("una vencida no vuelve a aprobada: %+v", d)
	}
}

// Rechazada es terminal: un evento posterior no la cambia.
func TestRechazadaEsTerminal(t *testing.T) {
	s := Abrir(t.TempDir())
	_ = s.Pedir(Pedido{Recibo: "r-1", Accion: "terraform apply", Via: ViaPregunta, Cuando: t0})
	_ = s.Resolver("r-1", Rechazada, "user:diego", t0.Add(time.Minute))
	_ = s.Resolver("r-1", Aprobada, "user:diego", t0.Add(2*time.Minute))
	if d := s.Estados(t0.Add(time.Hour))["r-1"]; d.Estado != Rechazada {
		t.Fatalf("rechazada es terminal: %+v", d)
	}
}

// Resolver algo que nunca se pidió es un error: no hay transición desde la nada.
func TestResolverSinPedidoEsError(t *testing.T) {
	s := Abrir(t.TempDir())
	if err := s.Resolver("r-x", Aprobada, "user:diego", t0); err == nil {
		t.Fatal("sin pedido no hay qué resolver")
	}
}
