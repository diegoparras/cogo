<h1 align="center">COGO</h1>

<p align="center">
  <b>La memoria con semáforo de confianza para construir software con IA<br>
  + el guardián que radiografía lo que un modelo te dice.</b>
</p>

<p align="center">
  Cada cosa que sabés de tu proyecto, con un color que dice cuánto podés confiar en ella.<br>
  Y cada turno de un LLM, con un color que dice cuánto te está empujando.
</p>

<p align="center">
  <a href="https://github.com/diegoparras/cogo/actions/workflows/docker.yml"><img alt="CI" src="https://github.com/diegoparras/cogo/actions/workflows/docker.yml/badge.svg"></a>
  <img alt="Go 1.25" src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white">
  <img alt="MIT" src="https://img.shields.io/badge/licencia-MIT-black">
  <img alt="MCP" src="https://img.shields.io/badge/MCP-stdio%20%2B%20HTTP-6E56CF">
  <img alt="firmada" src="https://img.shields.io/badge/imagen-firmada%20con%20cosign-2ea44f">
</p>

<p align="center"><sub><b><a href="README.md">🇬🇧 Read in English</a></b> · parte de la <b>Suite Escriba</b></sub></p>

---

## El problema

Cuando construís software —vos o un agente de IA (Claude Code, Cursor, Copilot)— vas juntando
"verdades": *"la base está en tal host"*, *"el bug lo causa X"*, *"decidimos Y"*.

Con el tiempo se **pudren**. Algunas nunca se verificaron, otras quedaron viejas, otras eran
una corazonada que alguien tipeó a las 2 de la mañana. Y acá está lo que duele:

> **Todas parecen igual de ciertas.**

Entonces actuás sobre una suposición creyendo que es un hecho. Peor: tu agente también, con
total seguridad y a velocidad de máquina.

## Qué hace COGO

COGO guarda ese conocimiento como **notas Markdown** y a cada una le pone un **color de
confianza que él mismo calcula**:

| | | |
|---|---|---|
| 🟢 | **verde** | verificado — podés confiar |
| 🟡 | **amarillo** | probable — falta confirmarlo |
| 🔴 | **rojo** | suposición — no te fíes |

**El color no lo elegís vos.** COGO lo deriva de cuatro cosas: ¿hay **evidencia**?, ¿se
**verificó**?, ¿está **fresco** (las cosas caducan)?, ¿**depende** de algo dudoso? Por eso no
se puede falsear: nadie puede pintar una nota de verde porque le parece.

Vive en un bloque computado que los agentes tienen prohibido escribir:

```yaml
# ---- computed by COGO · do not edit ----
confidence: red
color_reason: no observed or reported evidence
```

## Cómo se ve en la práctica

Estás debuggeando:

1. Anotás *"el worker no llega a Redis"* — es una corazonada, sin evidencia → 🔴 **roja**
2. Mirás los logs, encontrás la prueba, la sumás como evidencia → 🟡 **amarilla** (tenés
   evidencia, pero todavía no corriste el test que lo confirma)
3. Corrés el test, da bien, apretás **verificar** → 🟢 **verde**
4. La semana que viene le pedís ayuda a Claude. Claude lee tus notas, ve que la de Redis está
   **verde** (la usa como hecho) y otra está **roja** (no se apoya en ella). No pierde un turno
   re-investigando lo que ya probaste, ni actúa sobre tu corazonada.

Eso es COGO: **una memoria con semáforo de confianza, para vos y para tus herramientas de IA.**

## Guard: la radiografía anti-manipulación

La otra mitad de COGO. Cuando chateás con un LLM no tenés forma de saber si esa respuesta tan
segura es lógica de verdad o **humo**, ni de darte cuenta cuando la conversación te va llevando
de a poco a algo que no estabas dispuesto a hacer. Eso tiene nombre: es el **jailbreak al
humano**.

Guard lee cada turno del modelo **con el manual del adversario en la mano**: una ontología de
**108 técnicas de manipulación** destiladas de las 6 disciplinas que estudiaron cómo llevar a
una persona contra su voluntad — persuasión (Cialdini, Kahneman), **interrogatorio policial y
militar** (técnica Reid, Army FM 2-22.3, Scharff), negociación (Harvard, Voss), **coerción y
reforma del pensamiento** (Lifton, Biderman), manipulación emocional (gaslighting, DARVO,
chantaje FOG) y retórica/propaganda (Frankfurt, Grice, Walton). Cada técnica con su fuente
real, cómo se ve *en un chat*, y su **contramedida**.

Corre **determinista y offline** por defecto: sin modelo, sin API key, sin que nada salga de tu
máquina. Si le enchufás un modelo, va más profundo.

**Veracidad (`xray`)** es el gemelo de Guard. En vez de *cuánto me está empujando esto*, mide
**cuánto de esta respuesta está realmente sostenido**: el hueco entre lo que un texto afirma y
lo que respalda.

## Arrancar (la pavada)

```bash
docker run -d -p 127.0.0.1:8080:8080 -v cogo-vault:/vault -e COGO_ALLOW_INSECURE=1 ghcr.io/diegoparras/cogo
```

Abrís <http://localhost:8080> y listo — el visor viaja **adentro** del binario. Sin base de
datos, sin build, sin nada más que instalar.

> `COGO_ALLOW_INSECURE=1` está OK acá porque el puerto queda atado a tu máquina. En un servidor
> **no** lo uses: poné `COGO_MCP_TOKEN` — ver la [guía de deploy](docs/deploy.md).

<details>
<summary><b>Sin Docker</b> — un solo binario, sin runtime</summary>

```bash
go install github.com/diegoparras/cogo/cmd/cogo@latest
cogo init && cogo serve -http 127.0.0.1:8080 -vault ./vault
```
</details>

## Conectarlo a tu agente

COGO habla **MCP** por stdio (local) y Streamable HTTP (remoto), así que sirve cualquier
cliente MCP — Claude Code, Codex, Copilot, OpenCode, Antigravity:

```json
{
  "mcpServers": {
    "cogo": { "command": "cogo", "args": ["serve", "-vault", "./vault"] }
  }
}
```

Lo que Claude aprende hoy, mañana lo lee Cursor: **el mismo vault.**

### Las 14 herramientas que gana tu agente

| herramienta | qué hace |
|---|---|
| `pack` | contexto coloreado de un tema **antes de actuar** — lo rojo va en cuarentena |
| `search` | busca notas por significado (embeddings) o por palabra (BM25) |
| `open` | una nota, con su color recién computado |
| `capture` | registra un hallazgo — exige evidencia y check, y no acepta que le pases el color |
| `verify` | marca el check como pasado hoy y re-colorea |
| `archive` · `restore` | saca una nota del grafo sin destruirla, y la trae de vuelta |
| `remove` | borra de verdad — solo para basura genuina; deja una lápida |
| `stash` | guarda un artefacto por hash de contenido → lo citás como `artifact://<sha256>` |
| `recall` | re-anclarse tras una compactación de contexto, o ponerse al día con otro agente |
| `reflect` | contás qué hiciste; COGO te propone notas graduadas que vale la pena guardar |
| `lease` | tomás un lease con TTL sobre un recurso antes de una migración, un deploy o una edición masiva |
| `guard` | radiografía de manipulación sobre un turno del modelo |
| `xray` | radiografía de veracidad sobre una respuesta |

> **Memoria compartida entre máquinas.** Con MCP sobre HTTP + token, cualquier agente en
> cualquier máquina lee y escribe el mismo vault. `recall` es el cursor que lo convierte de
> archivo en canal: le devolvés el cursor que te dio y te trae **solo lo que cambió** desde
> entonces, más un cursor nuevo.

## El visor

Siete paneles, embebidos en el binario:

- **Vault** — un índice de verdad: búsqueda BM25, filtros buscables, rango de fechas de
  creación, paginador. Cada nota muestra cuándo nació, cuándo se verificó y cuándo vence.
- **Vigencia** — lo vencido y lo que está por vencer. Las cosas caducan.
- **Pack** — el bundle de contexto coloreado, exactamente como lo recibe un agente.
- **Grafo** — cómo se conecta tu conocimiento, y cómo el rojo se propaga por las dependencias.
- **Revisión** — enlaces rotos, notas vencidas y (con modelo) contradicciones entre notas.
- **Guard** · **Veracidad** — los dos motores, con su evidencia.

Más: un editor Markdown que **recomputa el color en vivo mientras escribís**, un **explorador
de GitHub** con mapa de confianza sobre tu repo, un gestor de instrucciones de agentes
(`AGENTS.md`, `CLAUDE.md`…), un log de auditoría descargable y podable, gestión de múltiples
tokens y exportación del vault en un clic.

## Evidencia que se puede volver a chequear

La evidencia no es una sensación, es una referencia que COGO puede ir a verificar de nuevo:

```yaml
evidence:
  - kind: file_read                                  # 9 tipos, de test_result
    ref: worker.go:12                                #  hasta hypothesis y absence
  - kind: command_output
    ref: github://acme/api@main/internal/db.go:88    # anclada al SHA del blob
  - kind: direct_log
    ref: artifact://9f2a…                            # dirección por contenido, inmutable
```

Las referencias a GitHub quedan ancladas al **SHA del blob**: si el archivo cambia upstream,
COGO ve la deriva y la nota cae a amarilla hasta que la re-verifiques. Los artefactos se
guardan bajo su **SHA-256** —en disco o en Cloudflare R2—, así que `verify` **recomputa** el
hash en vez de confiar en una cita que se pudre. Un **guard de secretos** corre antes de
guardar nada y, por defecto, se niega.

## Cómo se computa el color

```
confianza = min( evidencia , frescura , dependencia más débil , contradicción )
```

Una nota es verde solo cuando **nada** la empuja para abajo. La evidencia define el techo:
observada (un log, un comando, un test, un archivo) puede llegar a verde; reportada o inferida
tapa en amarillo; sin evidencia, rojo. La frescura decae por tipo — un comando dura 30 días,
una decisión de arquitectura 180. Cada color trae su `color_reason`, así que siempre podés
auditar **por qué**.

## Hecho para que le puedas creer

- **Un binario estático.** Go 1.25, `CGO_ENABLED=0`, imagen `scratch` (~12 MB), assets
  embebidos. Sin base de datos, sin runtime, sin `node_modules`.
- **Offline por defecto.** Todo lo que toca la red —modelos, embeddings, OIDC, R2, GitHub— es
  un accesorio opcional. El núcleo nunca llama a casa.
- **Cadena de suministro verificable.** Las imágenes se firman con **cosign** (Sigstore sin
  llaves) y llevan **provenance SLSA**. Los tests, `go vet` y `gofmt` bloquean cada publicación.
- **Tus datos son tuyos.** Las notas son archivos Markdown en una carpeta que es tuya. Borrás
  COGO y seguís teniendo todo, legible en cualquier editor de texto y versionable en git.

## En qué se diferencia

COGO no es la primera herramienta que guarda lo que sabés, ni siquiera la primera que lo
verifica. Notion y Confluence permiten que una persona marque una página como verificada con
fecha de vencimiento. Copilot Memory, de GitHub, ancla hechos a citas de código y las
re-chequea contra la rama actual. Varios proyectos de memoria para agentes tienen un campo
`confidence`.

Siendo preciso sobre qué es lo realmente distinto:

| | Notas<br>(Obsidian, Notion) | Memoria de agentes<br>(mem0, Zep, Letta…) | Copilot<br>Memory | **COGO** |
|---|:---:|:---:|:---:|:---:|
| Guarda lo que sabés | ✅ | ✅ | ✅ | ✅ |
| Marca qué está verificado | a mano | — | ✅ | ✅ |
| Lo decide el sistema, no el modelo | — | — | ✅ | ✅ |
| **Tres niveles, no sí/no** | — | — | — | ✅ |
| Re-chequea la evidencia | — | — | ✅ | ✅ |
| **La duda se propaga por dependencias** | — | — | — | ✅ |
| **Le dice al agente qué *no* usar** | — | — | — | ✅ |

Las tres que sí son nuestras:

- **Un color que el modelo tiene prohibido escribir.** Donde otras herramientas tienen un
  campo de confianza, suele completarlo el propio LLM — y una vez escrito no cambia nunca
  más. COGO lo computa desde la evidencia, y lo recomputa cada vez que mirás.
- **Duda que se propaga.** Todos los demás resuelven las contradicciones *de a pares* y ahí
  terminan. Un repaso de 435 trabajos sobre memoria de agentes lo nombra como problema
  abierto: *"la supersesión es local; los registros derivados no se vuelven a examinar"*. El
  `min()` sobre la dependencia más débil de COGO es exactamente esa pieza que falta.
- **Cuarentena en vez de filtro.** Los otros sistemas usan la verificación para *esconderle*
  lo dudoso al agente. COGO se lo entrega etiquetado como suposición y con la instrucción de
  no actuar sobre eso. Esconderlo implica que el agente no sabe lo que no sabe.

## Filosofía

> No saber no es una forma menor de saber. Es otra cosa, y tiene que estar a la vista.

La regla de hierro de COGO es que **el sistema nunca afirma más de lo que puede sostener**. El
color no es una etiqueta que alguien aplica; es una consecuencia de la evidencia. Eso es lo
único que lo hace valer algo — para vos, y frente a un modelo al que le encantaría decirte
exactamente lo que querés escuchar.

## Documentación

| | |
|---|---|
| [Instalación](docs/instalacion.md) | ponerlo a andar, paso a paso |
| [Deploy](docs/deploy.md) | tu compu, un servidor, o todo un equipo |
| [Manual](docs/manual.md) | cómo usarlo de verdad |
| [Para agentes de IA](docs/COGO-para-agentes.md) | ponele esto adelante a tu agente |
| [Motor de autonomía](docs/motor-autonomia.md) | Guard, en profundidad |
| [Motor de veracidad](docs/motor-veracidad.md) | xray, en profundidad |
| [Seguridad](docs/seguridad.md) | modelo de amenaza y endurecimiento |
| [Fundamento teórico](docs/fundamento-teorico.md) | por qué la regla de hierro |

## Licencia

MIT — Diego Parrás, CeMIACE / SEUBES / FCE-UBA. Parte de la **Suite Escriba**.
