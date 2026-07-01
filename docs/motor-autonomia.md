# COGO — Motor de Autonomía (radiografía retórica)

## Qué es

Toma un **turno del modelo** y, en vez de obedecerlo, expone la **presión que ejerce
sobre tu autonomía**: cuánto te empuja a cruzar tu **mandato** —lo que declaraste que
NO estás dispuesto a hacer o creer—. Devuelve una **radiografía por táctica** + un
veredicto (🟢/🟡/🔴).

Es el gemelo del [motor de veracidad](motor-veracidad.md):

- **veracidad** → ¿esto es **humo**? (¿es verdad?)
- **autonomía** → ¿esto me **empuja**? (¿respeta mi voluntad?)

No juzga "el mundo" ni "la verdad". Lee la **retórica** con las herramientas que la
psicología social, la negociación, el interrogatorio policial y militar, los estudios
sobre coerción y la pragmática desarrollaron para nombrar cómo se lleva a una persona a
actuar contra su disposición.

## La regla de hierro (teeth, no teatro)

> **Ningún modelo decide "esto es manipulación".** Los *teeth* son **deterministas**:
> léxico, actos de habla, estructura y —sobre todo— los **recibos** (NLI sobre la
> transcripción inmutable → gaslighting y deriva mecánicos). Un LLM solo **propone** qué
> técnica de la ontología matchea y formula la **pregunta crítica**; **nunca dicta** que
> te están manipulando. Y **el humano siempre decide**: el motor **no censura, inocula**.
> Un LLM juzgando si otro LLM te manipula = teatro, **prohibido** como oráculo final.

## El mandato (la referencia obligatoria)

"Manipulación" y "persuasión legítima" son **indistinguibles** sin una referencia de qué
NO estás dispuesto a hacer. En lenguaje de negociación, tu mandato es tu **BATNA**. El
motor mide **deriva respecto al mandato**, no "malas palabras". Sin mandato declarado,
degrada a **modo informativo** (nombra tácticas, no dicta veredicto de autonomía).

## La ontología (el corazón)

Tesis: **todo lo que se sabe sobre llevar a alguien contra su voluntad vive en 6
disciplinas adversariales, y cada una nació junto con su doctrina defensiva.** El motor
codifica el manual ofensivo como **ontología de detección** y entrega la defensa en
tiempo real = **inoculación** (McGuire; van der Linden, *prebunking*).

| # | Disciplina | Aporta | Defensa apareada |
|---|---|---|---|
| 1 | **Persuasión / psicología social** | Cialdini, ELM, sesgos, secuencias (FITD/DITF/low-ball) | Inoculación; forzar ruta central |
| 2 | **Interrogatorio policial + militar (HUMINT)** | Reid, Kassin (min/maximización), FM 2-22.3, Scharff | SERE: rechazar el binario, gray-rock |
| 3 | **Negociación** | Harvard (BATNA), Voss, Schelling, tácticas sucias | Conocer tu BATNA, "ir al balcón", nombrar la táctica |
| 4 | **Coerción / reforma del pensamiento** | Lifton, Hassan (BITE), Biderman, Skinner | Exit-counseling: contrastar afuera, anclar al mandato |
| 5 | **Manipulación emocional (dark psychology)** | Gaslighting, DARVO, FOG, dark patterns | Los **recibos**: el log refuta el gaslighting |
| 6 | **Propaganda / retórica / humo** | Frankfurt, Grice, IPA, Walton, modelo Milton | Preguntas críticas socráticas; exigir falsabilidad |

Ontología en `internal/suasion/ontology/*.yaml` (~108 técnicas); esquema en
`internal/suasion/ontology/_schema.md`. Cada técnica trae: fuente, definición, mecanismo,
**`in_llm`** (cómo se ve en un chat — el diferencial), detectores, preguntas críticas,
contramedida + inoculación, `fp_guard` (cuándo es uso legítimo, para no sobre-alarmar) y
`maps_to`.

## Los cuatro ejes del veredicto

- **veracidad** — lo cubre el [motor de veracidad](motor-veracidad.md) (el humo).
- **presión** — intensidad de influencia/coerción (Cialdini, Biderman, FOG).
- **autonomía** — deriva respecto al mandato (loading-the-language, falso binario, deriva de marco).
- **asimetría** — quién dirige a quién (iniciativa, control del turno).

## El pipeline (etapas)

| # | Etapa | Qué hace | Tier |
|---|-------|----------|------|
| 1 | Mandato/BATNA | carga la referencia del usuario (qué NO está dispuesto) | local |
| 2 | Léxico + acto de habla | marcadores deterministas por técnica | **determinista** + local |
| 3 | Recibos (NLI sobre transcripción) | contradicción / gaslighting / deriva | **determinista** |
| 4 | Match de técnica | mapea el turno a la ontología (6 disciplinas) | local (+ fuerte opcional) |
| 5 | Trayectoria | presión acumulada y desplazamiento de marco | **determinista** |
| 6 | Veredicto | color por eje y del turno | motor §4 — **determinista** |
| 7 | Inoculación | nombra la táctica + pregunta crítica + contramedida | render |

## Los tres tiers (y qué modelo para qué)

- **Tier 0 — Determinista (sin modelo).** Léxico, **recibos** (NLI), trayectoria, cómputo
  del veredicto. **Acá están los teeth.**
- **Tier 1 — Modelo local chico.** Clasificar el acto de habla y matchear técnicas claras
  (tareas *narrow*). **Gemma 2 9B** / **Qwen 2.5 7B** vía Ollama (multilingüe, clave en
  español). Privado, offline, por turno.
- **Tier 2 — Modelo fuerte (opcional).** Lo difícil: el **steelman del opuesto** (segunda
  opinión adversaria) y el match de tácticas sutiles. **Phi-4** local o un grande por
  **OpenRouter**. **Solo PROPONE; nunca dicta.**
- **Prohibido:** cualquier modelo como oráculo final de "te están manipulando".

> El plumbing de modelos **ya existe**: `internal/llm` habla OpenAI-compatible → Ollama
> **y** OpenRouter con la misma config. Cero código nuevo para los modelos.

## La superpotencia del proxy (los recibos)

Ser el cable da algo que el humano solo no tiene: **el log completo e inmutable**. El
gaslighting depende de que no puedas verificar el pasado; COGO sí puede, y te muestra
**las dos citas, lado a lado**. La **deriva longitudinal** (presión acumulada, corrimiento
del marco turno a turno) también es medible **solo desde el cable**. Es el detector más
barato y más potente, y existe únicamente porque COGO es proxy.

## El veredicto (reusa el §4)

El color del turno se **computa, no se opina**:

```
peor( presión , deriva-vs-mandato , asimetría )   // por eje, y total
```

- 🟢 = sin presión relevante / dentro del mandato.
- 🟡 = persuasión presente, declarada, sin cruzar tus líneas.
- 🔴 = empuja a cruzar el mandato, hay coerción, o hay **recibo** de gaslighting/deriva.

Es el mismo lattice de COGO — ahora alimentado por la **radiografía retórica**.

## Arquitectura (qué reusa)

- `internal/core` (motor de color §4) → computador de veredicto. **Ya está.**
- `internal/llm` → modelos (Ollama local + OpenRouter). **Ya está.**
- `vault` / transcripción → baseline (recibos, coherencia). **Ya está.**
- **Nuevo:** `internal/suasion` (el pipeline) + **ontología `go:embed`** + lexicón de marcadores.
- **Nuevo tool MCP:** `guard(turn, mandato)` — el agente se "cachea" a sí mismo antes de
  devolver una respuesta; y un endpoint web para pegar una conversación a mano.

## Plan de build (teeth primero)

1. **Fase 1 — Inoculador léxico + recibos** (sin modelo): etapas 1–3 + 6. Determinista.
   Ya nombra tácticas y caza gaslighting/deriva con citas. **Esto solo ya es valioso y honesto.**
2. **Fase 2 — Match de técnica con modelo local:** etapa 4 (Tier 1) + trayectoria (etapa 5).
3. **Fase 3 — Segunda opinión adversaria (steelman):** Tier 2, opcional.
