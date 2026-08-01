<h1 align="center">COGO</h1>

<p align="center"><b>La memoria con semáforo de confianza para construir software con IA<br>+ el guardián que radiografía lo que un modelo te dice.</b></p>

<p align="center">Cada cosa que sabés de tu proyecto, con un color que dice cuánto podés confiar en ella.<br>Y cada turno de un LLM, con un color que dice cuánto te está empujando.</p>

---

## El problema

Cuando construís software —vos o un agente de IA (Claude Code, Cursor, Copilot)— vas
juntando "verdades": *"la base está en tal host"*, *"el bug lo causa X"*, *"decidimos Y"*.
Con el tiempo se **pudren**: algunas nunca se verificaron, otras quedaron viejas, otras
eran una corazonada. El problema es que **todas parecen igual de ciertas** — y entonces
actuás (o el agente actúa) sobre una suposición creyendo que es un hecho.

## Qué hace COGO

COGO guarda ese conocimiento como **notas Markdown**, y a cada nota le pone un
**color de confianza que él mismo calcula**:

| | | |
|---|---|---|
| 🟢 | **verde** | verificado — podés confiar |
| 🟡 | **amarillo** | probable — falta confirmarlo |
| 🔴 | **rojo** | suposición — no te fíes |

El color **no lo elegís vos**: COGO lo deriva de cuatro cosas — ¿hay **evidencia**?, ¿se
**verificó**?, ¿está **fresco** (las cosas caducan)?, ¿**depende** de algo dudoso? Por eso
es auditable y no miente: nadie puede pintar una nota de verde "porque sí".

## Cómo se ve en la práctica

Estás debuggeando:

1. Anotás *"el worker no llega a Redis"* — es una corazonada, sin evidencia → 🔴 **roja**.
2. Mirás los logs, encontrás la prueba, la sumás como evidencia → 🟡 **amarilla** (tenés
   evidencia, pero todavía no corriste el test que lo confirma).
3. Corrés el test, da bien, apretás **"verificar"** → 🟢 **verde**.
4. La semana que viene le pedís ayuda a Claude. Claude lee tus notas, ve que la de Redis
   está **verde** (la usa como hecho) y otra está **roja** (no se apoya en ella). No pierde
   tiempo re-investigando ni actúa sobre la corazonada.

Eso es COGO: **una memoria con semáforo de confianza, para vos y para tus herramientas de IA.**

## Guard: la radiografía anti-manipulación

La otra mitad de COGO. Cuando chateás con un LLM —cualquiera— no tenés forma de saber si
esa respuesta tan segura es lógica de verdad o **humo**, ni de darte cuenta cuando la
conversación te va **llevando de a poco a algo que no estabas dispuesto a hacer**. Eso
tiene nombre: es el *jailbreak al humano*.

**Guard lee cada turno del modelo con el manual del adversario en la mano.** Adentro trae
una **ontología de 108 técnicas de manipulación** destiladas de las 6 disciplinas que
estudiaron cómo llevar a una persona contra su voluntad — persuasión (Cialdini, Kahneman),
**interrogatorio policial y militar** (técnica Reid, Army FM 2-22.3, Scharff), negociación
(Harvard, Voss), **coerción y reforma del pensamiento** (Lifton, Biderman), manipulación
emocional (gaslighting, DARVO, chantaje FOG) y retórica/propaganda (Frankfurt, Grice,
Walton). Cada técnica con su fuente real, cómo se ve *en un chat*, y su **contramedida**.

Con un modelo conectado detecta el **89%** de los turnos manipulativos de un corpus ciego
de 280 casos, sin marcar en rojo **ni uno solo** de los 132 benignos. Sin modelo, el piso
determinista es 32% — [la tabla completa, abajo](#cuánto-detecta-los-números).

Cómo funciona:

1. **Declarás tu mandato**: tu objetivo y tus líneas rojas (*"no renuncio sin otra oferta
   firmada"*). Queda guardado en el vault. Sin mandato, manipulación y persuasión legítima
   son indistinguibles — COGO entonces solo nombra técnicas, sin veredicto.
2. Pegás el turno (y la conversación previa) → **radiografía coloreada**: 🟢 sin señales,
   🟡 persuasión presente, 🔴 hay *mecánica* — el turno empuja sobre tu línea roja, o hay
   **recibos**.
3. **Los recibos** son la superpotencia: como COGO ve la transcripción, cuando el modelo
   niega lo que dijo (*"yo nunca dije que renuncies"*) COGO encuentra el turno donde SÍ lo
   dijo y te muestra **las dos citas, lado a lado**. El gaslighting deja de ser tu palabra
   contra la suya.
4. Cada táctica detectada llega con sus **preguntas críticas** y su contramedida — el motor
   **no censura al modelo: te inocula a vos**. Te muestra, vos decidís.

La regla de hierro: **ningún modelo dicta "te están manipulando"**. Los dientes son
deterministas (marcadores, recibos, trayectoria); los modelos solo *proponen* — y toda
propuesta se verifica contra el texto literal o se descarta. El *porqué* de esta regla —el
marco de la **alteridad arquitectónica**, común a los dos motores— está en
[`docs/fundamento-teorico.md`](docs/fundamento-teorico.md). Con un modelo conectado se
suman dos tiers opcionales: propuestas estructurales (el falso binario de Reid, que ningún
diccionario ve) y el **steelman adversario** — otro modelo argumentando a propósito el lado
que el turno no te mostró.

Se usa desde la pestaña **Guard** del visor, o desde cualquier agente vía el tool MCP
`guard`. La ontología completa vive en
[`internal/suasion/ontology/`](internal/suasion/ontology/) y el diseño en
[`docs/motor-autonomia.md`](docs/motor-autonomia.md) (su gemelo epistémico:
[`docs/motor-veracidad.md`](docs/motor-veracidad.md)).

### Cuánto detecta (los números)

Un detector que no publica su tasa de acierto te está pidiendo fe. Estos son los
nuestros, y los podés reproducir vos.

El corpus son **280 casos escritos por agentes que nunca vieron los detectores**
—148 turnos manipulativos y 132 benignos—, así que la referencia es la intención de
quien los escribió y no los marcadores del motor: el número no es circular. Entre
los benignos hay 75 **trampas**: turnos honestos que *suenan* a manipulación —
urgencia real, autoridad legítima, empatía sincera.

| | sin modelo (default) | con modelo conectado |
|---|---|---|
| Manipulativos detectados | 47/148 (32%) | **131/148 (89%)** |
| — sólo los **difíciles** (parafraseados) | 26/98 (27%) | 81/98 (83%) |
| Benignos marcados 🔴 **rojo** | **0/132** | **0/132** |
| Benignos que quedaron limpios | 121/132 (92%) | 120/132 (91%) |

La fila que más nos importa es la tercera: **ningún turno benigno terminó en rojo**,
con modelo o sin él, y las 75 trampas lo esquivaron enteras. Un detector que grita
lobo se apaga a la semana; preferimos que se nos escape uno antes que hacerte
desconfiar de una conversación sana.

**Ahora leé bien la primera columna: el default corre al 32%.** El modelo viene
apagado de fábrica —COGO es determinista y standalone— y en ese modo el léxico es
un ancla barata y muy precisa, pero de alcance corto: atrapa el cliché, no la
paráfrasis. Un 🟢 verde sin modelo significa *"no vi nada"*, **no** *"acá no hay
nada"*. Para el 89% hay que conectar un modelo (`COGO_LLM_*`, o Ajustes → Modelo IA
en el visor).

```bash
# columna 1 — determinista, gratis, 3 segundos
go test ./internal/suasion/ -run TestRedTeamDeterministic -v

# columna 2 — con modelo (medido con gpt-4o vía OpenRouter: 233 llamadas, ~4 min, ~USD 1,50)
export COGO_LLM_BASE_URL=https://openrouter.ai/api/v1
export COGO_LLM_MODEL=openai/gpt-4o
export COGO_LLM_API_KEY=sk-or-...
RUN_TIER1=1 go test ./internal/suasion/ -run TestRedTeamTier1 -v -timeout 45m
```

> **Próximo.** Que el Guard cruce lo que el modelo te afirma contra tus notas ya
> verificadas: *"te está afirmando X, y tu nota verde del martes —la del log— dice
> Y"*. Es la unión de las dos mitades de COGO y todavía no está.

### Veracidad: ¿esto es sólido o es humo?

El gemelo del Guard. Donde el Guard pregunta *"¿me está empujando?"*, la pestaña
**Veracidad** (tool MCP `xray`) pregunta *"¿esta respuesta se sostiene?"*. Pegás la
respuesta de un modelo y COGO la **radiografía frase por frase**, sin modelo, de
forma determinista: mide el **compromiso** (¿hedged o afirmado con fuerza?), la
**evidencia** (¿observada, reportada, o ninguna?) y si es **falsable** (una opinión
disfrazada de hecho). Una afirmación fuerte y sin fundamento sale 🔴; una sólida con
evidencia observada, mejor. Es la Fase 1 (el piso determinista) del *motor de
veracidad* — [`docs/motor-veracidad.md`](docs/motor-veracidad.md).

## Editar una nota cambia el color (es el punto)

El semáforo refleja el estado actual de la nota, **siempre**. En el visor editás una nota y
COGO **recomputa el color en vivo mientras escribís** (lo ves antes de guardar):

- sumás evidencia observada → más verde
- la dejás envejecer → decae sola a amarillo y después a rojo
- cambiás la afirmación → se reinicia a "hay que re-verificar"
- apretás **"verificar"** (ya lo chequeé) → verde

## Arrancar (la pavada)

Primeros pasos para principiantes: **[docs/instalacion.md](docs/instalacion.md)**.
Guía de despliegue completa (todos los modos, cada variable, respaldo,
actualización, problemas comunes): **[docs/deploy.md](docs/deploy.md)**.

**En tu compu, con Docker** — un comando, y abrís el navegador:

```bash
docker run -d -p 127.0.0.1:8095:8080 -v cogo-vault:/vault \
  -e COGO_ALLOW_INSECURE=1 ghcr.io/diegoparras/cogo
```

→ <http://localhost:8095>. Ves tu vault pintado por confianza, capturás y editás notas,
todo desde la web. **Cero terminal.** (`COGO_ALLOW_INSECURE=1` está OK acá porque el puerto
queda atado a tu máquina. Para un servidor **no** lo uses: poné `COGO_MCP_TOKEN` — ver la guía.)

**Sin Docker** — un solo binario, sin runtime:

```bash
cogo serve -http 127.0.0.1:8080 -vault ./vault
```

**Conectarlo a tu agente (MCP)** — el mismo binario es un servidor MCP. En Claude Code,
un `.mcp.json`:

```json
{ "mcpServers": { "cogo": { "command": "cogo", "args": ["serve", "-vault", "./vault"] } } }
```

Desde ahí, en cualquier sesión pedís `pack "<tema>"` y obtenés contexto coloreado, o
`capture` un hallazgo. Lo que Claude aprende hoy, mañana lo lee Cursor: **el mismo vault.**

## Cómo se computa el color

```
confianza = min( evidencia , frescura , dependencia más débil , contradicción )
```

Una nota es **verde** solo cuando **nada** la empuja para abajo: evidencia observada, con un
check que pasó, fresca, todas sus dependencias verdes y sin contradicciones. Cada color trae
su `color_reason`, así que siempre podés auditar **por qué** quedó como quedó.

La **evidencia** define el techo del color: observada (un log, un comando, un test, un
archivo) puede llegar a verde; reportada o inferida tapa en amarillo; sin evidencia, rojo.
La **frescura** decae por tipo (un comando dura 30 días; una decisión de arquitectura, 180).

## Las tres caras, una sola lógica

| Cara | Para quién | Cómo |
|------|------------|------|
| **Visor web** | todos | `cogo serve -http :8080` → navegador (Vault · Vigencia · Pack · Grafo · Revisión · **Guard** · **Veracidad**) |
| **MCP** | tu agente (Claude, Codex, Cursor, Gemini…) | `cogo serve` (stdio) — tools: `pack` `search` `open` `capture` `verify` `archive` `restore` `remove` `recall` `reflect` `stash` `lease` `guard` `xray` |
| **CLI** | power users | `cogo add · pack · search · stale · verify · lint · agents` |

Es un **solo binario Go** (imagen Docker `scratch` de ~12 MB) que es las tres cosas a la vez.

> **Memoria compartida entre máquinas.** Con el MCP sobre HTTP + token, cualquier agente en cualquier máquina lee y escribe el mismo vault. `recall` es el **cursor** que lo vuelve un canal, no solo un archivo: la primera llamada devuelve el bundle que sostiene el proyecto (mandato + decisiones verdes) y un cursor; pasás ese cursor como `since` y `recall` te da **solo lo que cambió** desde entonces —sin releer todo—, más un cursor nuevo.

> **Evidencia que vive en GitHub.** Una nota puede citar un archivo del repo como `github://owner/repo@main/src/foo.go:42`. COGO baja el archivo por la API, confirma que la cita **existe** y guarda el **hash del blob**. Eso hace dos cosas: le devuelve los dientes al motor de color en una instancia **hosteada** (que no tiene tu working copy, así que hoy no puede verificar ninguna ruta local), y habilita la **frescura anclada a git** — la nota sigue verde **mientras el archivo citado no cambie**; si cambia en la rama, cae a amarilla pidiendo re-verificar. Citá un **commit fijo** y la evidencia es inmutable. Sin dependencias: 3 endpoints REST.

> **Artefactos con dirección por contenido.** `stash` guarda un artefacto (la salida completa de un comando, un CSV, un archivo chico) bajo la clave = **SHA-256 de su contenido** y te devuelve `artifact://<sha>` para citarlo como evidencia. Como la clave ES el hash, la referencia prueba que el artefacto no cambió: `verify` **recomputa** (existe y su hash matchea → la evidencia vale; desapareció → se rompe y el color cae) en vez de confiar en una cita que se pudre. Un **guard de secretos** corre antes de guardar y, por defecto, **se niega** a inmortalizar credenciales (opt-in a redactar). Standalone guarda en disco (`.cogo/artifacts`); con `COGO_R2_*` guarda en Cloudflare R2.

### Todo se maneja desde el visor (menú ⋮)

Para el que no quiere tocar la terminal, cada cosa operativa vive en el menú:

| | |
|---|---|
| **Conexiones MCP** | emitir/revocar tokens por app (con vencimiento y modo *solo lectura*) |
| **Papelera** | notas borradas — restaurar o borrar para siempre |
| **Auditoría MCP** | quién llamó a qué herramienta, cuándo y desde qué IP |
| **Raíces de evidencia** | contra qué carpeta se resuelve la evidencia de cada proyecto |
| **Exportar (backup)** | bajar todo el vault como zip (sin secretos) |
| **Instrucciones para agentes** | generar el `AGENTS.md`/`CLAUDE.md` que le enseña el protocolo a tu agente |
| **Ajustes · Modelo IA** | conectar un modelo (OpenRouter/Ollama) para contradicciones y Guard |

## Accesorios opcionales (apagados por default)

COGO es 100% determinista y standalone sin nada de esto. Se prenden por variable de entorno
y **nunca tocan el núcleo**:

| Accesorio | Se prende con | Para |
|---|---|---|
| **Modelo IA** (OpenRouter, Ollama, DeepSeek…) | `COGO_LLM_BASE_URL` + `COGO_LLM_MODEL` (o Ajustes en la GUI) | detectar **contradicciones** entre notas + los tiers de Guard |
| **Juez fuerte independiente** | `COGO_LLM_STRONG_BASE_URL` + `COGO_LLM_STRONG_MODEL` | que el **steelman** de Guard no comparta cerebro con el proponente |
| **Scrub Anonimal** | `ANONIMAL_URL` | que secretos/PII no entren al vault |
| **Login Lockatus (OIDC)** | `AUTH_MODE=federado` | federar con la Suite Escriba |

```bash
# ejemplo: detectar contradicciones con un modelo de OpenRouter
export COGO_LLM_BASE_URL=https://openrouter.ai/api/v1
export COGO_LLM_MODEL=deepseek/deepseek-chat
export COGO_LLM_API_KEY=sk-or-...
cogo lint
```

## CLI (para rosquear)

```bash
cogo init                 # crea un vault
cogo add nota.md          # valida, computa el color, la guarda (stdin si no hay archivo)
cogo pack "redis"         # arma un contexto coloreado para un tema (degrada el rojo)
cogo search "worker"      # lista: color · id · resumen (sin cuerpos)
cogo stale                # qué está vencido o por vencer
cogo verify <id>          # "ya lo chequeé": revalida y re-colorea
cogo lint                 # enlaces rotos, vencidas, y contradicciones (si hay modelo)
cogo agents --claude      # genera el CLAUDE.md/AGENTS.md que le enseña el protocolo a un agente
cogo install              # cablea COGO en el .mcp.json del agente (stdio; --http para remoto)
cogo serve -http :8080    # visor web + servidor MCP por HTTP
cogo serve                # servidor MCP por stdio
```

## Formato de una nota

```yaml
---
id: fisherboy-redis-hostname
type: bug                 # decision|bug|runbook|architecture|constraint|command|mistake
project: fisherboy
evidence:
  - kind: direct_log      # observada → puede llegar a verde
    ref: "api log 2026-06-27T14:03Z: connect OK to redis:6379"
check:
  test: "leer el env efectivo del worker; probar conectividad a fisherboy-redis:6379"
  status: not_run         # passed | failed | not_run
last_verified: 2026-06-27
depends_on: [fisherboy-redis-topology]
# ---- computed by COGO · do not edit ----
confidence: yellow
color_reason: "evidencia observada pero el check no pasó"
---

## Claim
El worker probablemente falla porque no resuelve el hostname interno de Redis.
```

El vault Markdown es la **única fuente de verdad**: portable, diffeable, sobrevive a que la
herramienta muera. Todo lo demás es un cliente fino o un caché reconstruible.

## Filosofía

Parte de la **Suite Escriba** (getescriba.com): self-hosted, Docker, interno por defecto,
MIT. **Standalone-first**: un `docker run` y anda; la federación es un accesorio. Simple por
sustracción — el día que le crezca un kanban, falló.

## Build / desarrollo

```bash
go build -o cogo ./cmd/cogo     # un binario estático, sin CGO
go test ./...                   # toda la suite
docker build -t cogo .          # imagen scratch (~12 MB)
```

## Licencia

[MIT](LICENSE) · Diego Parrás · Ecosistema Escriba.
