# Las dos radiografías: Guard y Xray

> Este documento era la Parte VII del [manual](manual.md). Está aparte porque
> Guard y Xray son **otra cosa** que la memoria: no leen ni escriben el vault,
> miran el diálogo. Se pueden dejar afuera del servidor MCP con
> `cogo serve -sin-radiografias` y el resto de COGO no se entera.

Hasta acá COGO responde *"¿cuánto vale lo que sé?"*. Guard y Xray responden otra
pregunta, sobre el mismo turno de conversación: **"¿esto que me está diciendo el
modelo, me está empujando?"** y **"¿lo que afirma lo puede sostener?"**.

Son la parte de COGO que mira **hacia el otro lado**: no a la memoria, sino al
diálogo.

## Xray — la radiografía de veracidad

Determinista. **No usa ningún modelo y no ejecuta nada.**

Toma una respuesta de IA y, claim por claim, expone la distancia entre **cuánto
compromete el lenguaje** y **cuánto respaldo declara**:

```
"Sin duda el pool aguanta 200 conexiones."     ← afirma fuerte, no declara base
"Probablemente convenga migrar."               ← opinión con forma de opinión: OK
"El test pasó."                                ← factual, sin fuente
```

Lo que marca:

- claims afirmados con fuerza y sin ninguna base declarada,
- opiniones vestidas de hechos,
- afirmaciones fácticas sin fuente.

**Nunca dice "esto es verdad".** No puede: verificar exige ejecutar algo, y eso es
el runner. Acá el techo de un claim es amarillo, y **el valor está en cazar los
rojos** — el humo, no el fuego.

## Guard — la radiografía de manipulación

Guard mide **presión de influencia** sobre un turno del modelo, con cuatro ejes:

| Eje | Qué mide |
|---|---|
| **veracidad** | el humo — lo cubre Xray |
| **presión** | intensidad de influencia y coerción |
| **autonomía** | deriva respecto a tu mandato |
| **asimetría** | quién dirige a quién: iniciativa, control del turno |

### La ontología

4.588 líneas de YAML, **108 tácticas** catalogadas en seis disciplinas:

```
persuasión · interrogatorio · negociación · coerción · dark psychology · retórica
```

Cada táctica trae su definición, sus marcadores y la **pregunta crítica** que la
desarma. El motor no te dice "te están manipulando": te nombra la técnica, te
muestra la cita textual que la disparó, y te da la pregunta.

### La regla de hierro

> **Ningún modelo decide "esto es manipulación".**
>
> Los dientes son **deterministas**: léxico, actos de habla, estructura y —sobre
> todo— los **recibos**. Un LLM solo *propone* qué técnica de la ontología encaja y
> formula la pregunta crítica; **nunca dicta** el veredicto.
>
> Un LLM juzgando si otro LLM te manipula es teatro, y está prohibido como oráculo
> final.

### Los recibos

Es la superpotencia de estar en el medio de la conversación. COGO tiene la
**transcripción inmutable**, así que puede contrastar lo que el modelo dice *ahora*
contra lo que dijo *antes*.

Si el modelo niega haber dicho algo que está en la transcripción, eso no es una
opinión sobre su tono: es un hecho verificable. **Gaslighting y deriva de marco,
detectados mecánicamente.**

### El mandato

"Manipulación" y "persuasión legítima" son **indistinguibles** sin una referencia
de qué NO estás dispuesto a hacer. En lenguaje de negociación, tu mandato es tu
BATNA.

Guard mide **deriva respecto al mandato**, no "malas palabras". Sin mandato
declarado degrada a **modo informativo**: nombra tácticas, pero no dicta veredicto
de autonomía.

Se declara una vez, en el visor, y vive fuera de las notas — es estado privado, no
memoria compartida.

### Y no censura: inocula

El veredicto es para vos, no para bloquear al modelo. La apuesta es que **saber que
te están aplicando una técnica la desactiva**, y que un sistema que decide por vos
te deja peor parado que uno que te muestra lo que está pasando.

---

**Para ir más hondo**

- [`motor-autonomia.md`](motor-autonomia.md) — Guard: la ontología de 108 técnicas, los recibos, la deriva
- [`motor-veracidad.md`](motor-veracidad.md) — Xray: compromiso contra respaldo, afirmación por afirmación
- [`manual.md`](manual.md) — el resto de COGO: la memoria, el color, el registro
