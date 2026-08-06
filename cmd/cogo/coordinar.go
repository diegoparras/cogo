package main

import (
	"context"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/lease"
	"github.com/diegoparras/cogo/internal/presencia"
)

// La coordinación entre agentes, enchufada donde ya se llama.
//
// COGO no puede empujar: MCP es pregunta y respuesta, y no hay forma de
// interrumpir a un agente a mitad de turno. Pero el protocolo ya obliga a
// llamar ANTES de actuar —`pack` primero, `authorize` antes de tocar nada— y
// esos llamados son el canal.
//
// Así que el aviso viaja colgado de la respuesta, y llega exactamente cuando es
// accionable: cuando el agente está por hacer algo.

// quienesMas devuelve los otros agentes activos y los permisos vigentes.
//
// Un agente solo se entera de quien está EN SU MISMO PROYECTO. Avisar de alguien
// que trabaja en otro repo no es información, es ruido — y un aviso que aparece
// siempre entrena a no leer los avisos.
func quienesMas(ctx context.Context, dir, proyecto string) (otros []presencia.Agente, permisos []lease.Lease, yo string) {
	yo = auth.CallerCtx(ctx)
	minutos := pars.Entero("coordinacion.ventana_minutos")
	if minutos <= 0 {
		return nil, nil, yo // apagado
	}
	permisos = lease.Open(dir).List(time.Now())
	desde := time.Now().Add(-time.Duration(minutos) * time.Minute)
	activos := presencia.Ver(dir, desde)
	return presencia.EnProyecto(presencia.Otros(activos, yo), proyecto), permisos, yo
}

// avisoDeOtros arma el bloque que se le cuelga a una respuesta, o "" si no hay
// nada que decir.
func avisoDeOtros(ctx context.Context, dir, proyecto string) string {
	otros, permisos, yo := quienesMas(ctx, dir, proyecto)
	return presencia.Aviso(otros, permisos, yo, time.Now())
}

// aplicarChoque convierte un permiso ajeno en un rechazo.
//
// Es el único bloqueo de COGO que no habla de evidencia. El resto pregunta "¿te
// alcanza lo que sabés?"; este pregunta "¿ya lo está haciendo otro?" — y las dos
// respuestas tienen que ser sí para seguir. Verificar mejor no lo destraba:
// se destraba hablando con el otro, o esperando a que suelte.
func aplicarChoque(ctx context.Context, dir string, v accion.Veredicto, texto string) accion.Veredicto {
	if !pars.Bool("coordinacion.bloquear_por_permiso") {
		return v
	}
	yo := auth.CallerCtx(ctx)
	l, hay := presencia.Choque(texto, lease.Open(dir).List(time.Now()), yo)
	if !hay {
		return v
	}
	v.Autoriza = false
	v.Bloqueo = "BLOCKED BY ANOTHER AGENT — " + l.Holder + " holds the lease `" + l.Name +
		"` until " + l.Expires + ", and your action names it."
	if l.Note != "" {
		v.Bloqueo += " They said: " + l.Note
	}
	v.Bloqueo += "\nThis is not about your evidence: verifying more will not unblock it. " +
		"Wait for the lease, take it yourself once it is free, or tell the human that two agents are on the same thing."
	return v
}
