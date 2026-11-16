---
name: cogo
description: Usar en cualquier proyecto conectado a COGO. Antes de responder o cambiar algo, pedir contexto con pack; antes de una acción costosa o irreversible, authorize; después de compactar, recall; y capturar lo verificado sin escribir el color.
---

# COGO: la memoria con semáforo

Este proyecto usa **COGO** como memoria: notas con un **color de confianza computado** (verde/amarillo/rojo). El color ya viene calculado por COGO — no lo decidís vos, lo obedecés.

## Protocolo (obligatorio)

1. **Consultá antes de actuar.** Antes de responder o cambiar algo en este proyecto, pedí contexto a COGO con la herramienta MCP `pack` (o `search` para listar, `open` para una nota). Trae lo que ya se sabe, coloreado.
2. **Respetá el color.**
   - 🟢 **verde** = verificado. Podés apoyarte.
   - 🟡 **amarillo** = probable. Usalo con cautela y decí que es probable.
   - 🔴 **rojo** = **NO te apoyes**. Está en cuarentena (`pack` ya lo degrada a *do-not-rely*): es un supuesto sin evidencia, una cita rota, o una contradicción abierta. Tratalo como "esto puede estar mal".
3. **Capturá lo que verifiques.** Cuando confirmes algo nuevo, guardalo con `capture`: un *claim* declarativo + la evidencia (archivo, comando, log) + el `check` que lo probaría. No escribas el color: COGO lo computa a partir de la evidencia.
4. **No pises el verde.** Si ya existe una nota verde, no la sobrescribas a ciegas: verificala de nuevo o usá un id nuevo.
5. **El rojo no se "arregla" escribiendo.** Una contradicción o una cita rota se resuelve corrigiendo la nota o la evidencia, no cambiando el texto para que suene mejor.

## Antes de actuar: `authorize`

Antes de algo que cambia cosas afuera de tu respuesta y es costoso o irreversible —borrar, migrar, desplegar, publicar— preguntale a COGO con `authorize` si lo que sabés alcanza. Este plugin trae hooks que lo hacen solos antes de cada comando y cada edición, pero si vas a hacer algo que no pasa por esas herramientas, llamalo vos, con las notas en las que te apoyás o con `find_support: true`.

- Si contesta `NOT AUTHORIZED`, no lo rodees: contale a la persona qué falta, con el número de recibo, y dejá que decida.
- Si contesta `AUTHORIZED BY THE PERSON`, COGO no lo autorizaba y la persona lo aprobó: seguí, y decí que fue decisión suya.
- Si el diálogo de permiso te pregunta algo de parte de COGO, es la persona la que decide, no vos.

## Después de compactar: `recall`

Cuando el contexto se compacta se pierden las reglas. El plugin llama a `recall` solo después de compactar, retomar o limpiar la sesión; si no lo ves en el contexto, llamalo vos: trae el mandato, las líneas rojas y las restricciones verdes.
