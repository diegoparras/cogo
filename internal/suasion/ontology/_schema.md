# Ontología de manipulación — esquema

Esta carpeta es el activo diferencial de COGO: el **manual ofensivo** de cómo se lleva a
una persona a actuar contra su voluntad, codificado como datos para que el **motor de
autonomía** (`internal/suasion`) lo detecte en los turnos de cualquier modelo. Manifiesto:
`docs/motor-autonomia.md`.

Un archivo `*.yaml` por disciplina. Los archivos que empiezan con `_` son **metadatos** y
no se cargan como disciplina (`_manifest.yaml`, `_schema.md`). Cada archivo de disciplina:

```yaml
discipline: persuasion                       # enum (ver abajo)
display_name: "Persuasión e influencia social"
techniques:
  - id: persuasion.reciprocity               # <disciplina>.<snake_case>, único
    ...
```

## Campos de una técnica

| Campo | Oblig. | Qué es |
|---|:---:|---|
| `id` | sí | `<disciplina>.<snake>`, único y estable (es **la clave**) |
| `name` | sí | nombre legible |
| `aka` | no | sinónimos |
| `family` | no | sub-agrupación (p.ej. `reciprocity`, `fear`) |
| `source` | sí | `{author, work, year, note}` — **solo fuentes reales**; cada subcampo es opcional. `note` = procedencia real sin obra canónica citable (términos de literatura popular) |
| `definition` | sí | qué es la técnica |
| `mechanism` | sí | qué palanca cognitiva/social explota (por qué funciona) |
| `in_llm` | sí | **cómo se manifiesta en un chat humano↔LLM (el diferencial)** |
| `axes` | sí | ⊆ {presion, autonomia, asimetria, veracidad} |
| `detectors` | sí | lista de `{type, signal}` — cómo se dispara |
| `trajectory` | sí | `single` / `longitudinal` / `both` |
| `severity` | sí | `low` / `medium` / `high` / `critical` (default) |
| `critical_questions` | sí | preguntas socráticas al humano (estilo Walton) |
| `countermeasure` | sí | `{doctrine, move, inoculation}` — la defensa apareada |
| `fp_guard` | no | cuándo es uso **legítimo** (anti sobre-alarma) |
| `maps_to` | no | ids de técnicas relacionadas |

## Enums

- **discipline**: `persuasion` · `interrogation` · `negotiation` · `coercion` · `dark_psychology` · `rhetoric`
- **axes**: `presion` · `autonomia` · `asimetria` · `veracidad`
- **trajectory**: `single` (un turno) · `longitudinal` (entre turnos) · `both`
- **severity**: `low` · `medium` · `high` · `critical`
- **detector.type**:
  - `lexicon` — palabras / marcadores léxicos
  - `speech_act` — tipo de acto (directiva, aseveración, compromiso…)
  - `pragmatic` — presuposición / implicatura
  - `structure` — forma del mensaje (falso binario, favor→pedido…)
  - `trajectory` — patrón entre turnos
  - `nli_transcript` — contradicción contra lo dicho antes (los **recibos**)
  - `frame` — encuadre / marco
  - `meta` — sobre la relación (autoridad, dependencia, aislamiento)

## Principios de diseño

- **El diferencial es `in_llm`.** Cualquiera lista "reciprocidad"; el valor está en
  describir cómo la ejerce un **modelo en un chat**.
- **`fp_guard` para no gritar por todo.** La persuasión legítima existe; el motor solo
  alarma contra el **mandato** del usuario. El costo del falso positivo es alto.
- **La defensa está apareada.** Cada técnica ofensiva trae su contramedida e
  **inoculación** (prebunk): el motor no censura, inocula.
- **Los `id` son contrato.** El Go (`ontology.go`) y los `maps_to` dependen de ellos; no
  se renombran a la ligera.
