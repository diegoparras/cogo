# COGO

**La capa de memoria con color para construir software con agentes IA.**

COGO guarda lo que sabés de un proyecto como notas Markdown, y a cada nota le pone
un **color de confianza computado**. Tu agente —o vos— consume ese color en vez de
adivinar si algo es confiable.

- 🟢 **verde** — verificado
- 🟡 **amarillo** — probable
- 🔴 **rojo** — suposición, no te fíes

El color **no se escribe a mano**: COGO lo calcula a partir de la evidencia, de la
frescura (las cosas caducan) y de qué dependen las notas. Es auditable y no miente.
Esa es la única parte original; todo lo demás es Markdown en una carpeta de git.

## Arrancar (la pavada)

**Con Docker:**

```bash
docker run -p 8095:8080 -v cogo-vault:/vault ghcr.io/diegoparras/cogo
```

Abrí <http://localhost:8095> → ves tu vault pintado por confianza, capturás y
editás notas, todo desde el navegador. Nada de terminal.

**Sin Docker** (un solo binario, sin runtime):

```bash
cogo serve -http :8080 -vault ./vault
```

## Las tres caras, una sola lógica

| Cara | Para quién | Cómo |
|------|------------|------|
| **Visor web** | todos | `cogo serve -http :8080` → navegador |
| **MCP** | tu agente (Claude Code, Codex, Cursor, Gemini…) | `cogo serve` (stdio) |
| **CLI** | power users que quieran rosquear | `cogo add · pack · lint · …` |

Cualquier herramienta que hable MCP se conecta al **mismo** vault: lo que Claude
aprende hoy, mañana lo lee Cursor.

## Capturar una nota

Desde el visor: botón **+ Nueva nota**, escribís el claim, sumás evidencia, y ves
el color computado **en vivo** antes de guardar.

Desde la terminal:

```bash
cogo add nota.md          # valida, computa el color, la guarda
cogo pack "redis"         # arma un contexto coloreado para un tema
cogo verify <id>          # "ya lo chequeé": revalida y re-colorea
cogo lint                 # links rotos, vencidas, y contradicciones (si hay LLM)
```

## Cómo se computa el color (§4)

`confianza = min( evidencia , frescura , dependencia más débil , contradicción )`

Una nota es verde solo cuando **nada** la empuja para abajo: evidencia observada,
con un check que pasó, fresca, todas sus dependencias en verde y sin
contradicciones. Cada color trae el motivo (`color_reason`), así que siempre
podés auditar por qué quedó como quedó.

## LLM opcional (apagado por defecto)

COGO es 100% determinista sin ningún modelo. Si querés que detecte
**contradicciones** entre notas, apuntá un provider OpenAI-compatible —local
(Ollama) o remoto (OpenRouter, DeepSeek, Qwen…):

```bash
export COGO_LLM_BASE_URL=https://openrouter.ai/api/v1
export COGO_LLM_MODEL=deepseek/deepseek-chat
export COGO_LLM_API_KEY=sk-or-...
cogo lint
```

Solo salen al modelo las frases de *claim* de los pares candidatos, nunca el vault
entero. El servidor MCP no tiene red de salida: la única llamada al modelo vive en
`cogo lint`, que corrés a propósito.

## Filosofía

Parte de la [Suite Escriba](https://getescriba.com): self-hosted, Docker, interno
por defecto. El vault Markdown es la única fuente de verdad; todo lo demás es un
cliente fino o un caché reconstruible. Simple por sustracción: el día que le crezca
un kanban, falló.
