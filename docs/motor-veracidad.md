# COGO — Motor de Veracidad (radiografía epistémica)

## Qué es

Toma una **respuesta de IA** y, en vez de creerla, expone el **gap entre cuánto se
compromete el lenguaje y cuánto fundamento tiene** — y testea lo testeable. Devuelve
una **radiografía por afirmación** + un veredicto (🟢/🟡/🔴).

No verifica "el mundo". Lee el **lenguaje** de la respuesta con las herramientas que la
lingüística y la filosofía del lenguaje desarrollaron para eso, y solo dicta 🟢 cuando un
test ejecutable lo respalda.

## La regla de hierro (teeth, no teatro)

> **Ningún modelo decide "es verdad".** La verdad sale de un **test ejecutable** (leer el
> código, correr el comando, buscar la fuente) o queda **"no testeada"**. Los modelos solo
> hacen **análisis lingüístico acotado** y **proponen** tests. Un LLM juzgando si otro LLM
> dice la verdad = teatro, **prohibido**.

## El pipeline (9 etapas)

| # | Etapa | Qué hace | Corriente | Tier |
|---|-------|----------|-----------|------|
| 1 | Segmentar | parte la respuesta en afirmaciones atómicas | — | local |
| 2 | Marcadores epistémicos | hedges/modalidad ("probablemente", "seguro", "creo") → nivel de compromiso | modalidad (lingüística) | **determinista** |
| 3 | Acto de habla | ¿aseveración, especulación, directiva? | Austin / Searle | local |
| 4 | Evidencialidad | ¿declara fuente? observado / inferido / reportado / ninguna | evidencialidad | local + determinista (detecta refs) |
| 5 | Toulmin | ¿hay *grounds* y *warrant*, o claim pelado? | Toulmin | local |
| 6 | Falsabilidad + test | ¿qué lo probaría falso? ¿cuál es el test más barato? | Popper | **fuerte** (o local) |
| 7 | Ejecutar el test | corre el test si es ejecutable | — | **determinista (ejecución)** |
| 8 | Coherencia | ¿contradice algo ya verificado (el vault)? | coherentismo | determinista + local |
| 9 | Veredicto | computa el color de cada afirmación y del total | motor §4 de COGO | **determinista** |

## Los tres tiers (y qué modelo para qué)

- **Tier 0 — Determinista (sin modelo).** Marcadores epistémicos, detección de refs,
  ejecución del test, coherencia vs vault, cómputo del veredicto. **Acá están los teeth.**
- **Tier 1 — Modelo local chico.** Las etapas estructurales (segmentar, acto de habla,
  evidencialidad, Toulmin): tareas *narrow* de "analizá la estructura de esta oración",
  que un modelo de 2–9B hace bien. **Gemma 2 9B** o **Qwen 2.5 7B** (multilingüe — clave
  porque trabajás en español), vía Ollama. **Privado, offline, barato** (corre en CADA
  respuesta sin costo).
- **Tier 2 — Modelo fuerte (opcional).** Lo difícil: derivar un test de falsación filoso y
  la **refutación adversaria** (steelman del opuesto). **Phi-4 (14B) local** o un modelo
  grande por **OpenRouter** (DeepSeek, Qwen-72B, Claude). **Solo PROPONE; nunca dicta
  verdad.** Ocasional, no por oración.
- **Prohibido:** cualquier modelo como oráculo final de verdad.

> El plumbing de modelos **ya existe**: `internal/llm` habla OpenAI-compatible → Ollama
> (Gemma / Phi / Qwen local) **y** OpenRouter con la misma config. Cero código nuevo para
> los modelos; solo cambia la URL.

## Dónde corre el test (los ejecutores)

- **código / archivo:** lee o `grep` el repo. (*"el bug está en X"* → leé X)
- **comando:** corre un comando *whitelisted*, en sandbox.
- **web:** `fetch` a una fuente (como con la evidencialidad).
- **vault:** chequea coherencia contra lo ya verificado.
- **ninguno aplica → "no testeable acá".** Honesto: no inventa un 🟢.

## El veredicto (reusa el §4)

El color de cada afirmación **se computa, no se opina**:

```
min( evidencialidad , fundamento(Toulmin) , falsabilidad , resultado-del-test , coherencia )
```

- 🟢 verde = falsable + test corrido + pasó + coherente.
- 🟡 = afirmada con fundamento reportado/inferido, o falsable pero no testeada.
- 🔴 = no falsable (opinión), refutada, o sin fundamento.

Es el mismo lattice de COGO — ahora alimentado por la **radiografía**, no por tags a mano.

## Arquitectura (qué reusa)

- `internal/llm` → modelos (Ollama local + OpenRouter). **Ya está.**
- `internal/core` (motor de color §4) → el computador de veredicto. **Ya está.**
- `vault` → baseline de coherencia. **Ya está.**
- **Nuevo:** `internal/xray` (las 9 etapas) + ejecutores + lexicón de hedges.
- **Nuevo tool MCP:** `xray(answer)` — el agente se radiografía a sí mismo **antes de
  actuar**. Y un endpoint web para pegar una respuesta a mano.

## Plan de build (por fases, teeth primero)

1. **Fase 1 — El medidor de gap** (ya tiene teeth, sin ejecución): etapas 1–5 + 9.
   Determinista (hedges) + modelo local (estructura). Te dice *"esto se afirma fuerte pero
   sin fundamento declarado / es inferencia disfrazada / no es falsable"*. **Esto solo ya es
   valioso y honesto** — es lo que demostré a mano en la conversación.
2. **Fase 2 — Los teeth de verdad:** etapas 6–7. Derivación del test + ejecutores
   (código, web). Acá el 🟢 pasa a tener prueba.
3. **Fase 3 — Refutación adversaria + coherencia:** etapa 8 + Tier 2.
