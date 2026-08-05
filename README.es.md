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

### Las 16 herramientas que gana tu agente

| herramienta | qué hace |
|---|---|
| `pack` | contexto coloreado sobre un tema **antes de actuar** — lo rojo va en cuarentena |
| `authorize` | **preguntar si lo que sabe alcanza para lo que va a hacer** |
| `search` | encontrar notas por significado (embeddings) o por palabra (BM25) |
| `open` | una nota, con su color recién computado |
| `capture` | registrar un hallazgo — evidencia y check obligatorios, color no se acepta |
| `verify` | marcar el check como pasado hoy y re-colorear |
| `gap` | **registrar lo que el proyecto NO sabe, como pregunta abierta** |
| `archive` · `restore` | sacar una nota del grafo sin destruirla, y traerla de vuelta |
| `remove` | borrar de verdad — solo para basura genuina; deja lápida |
| `stash` | guardar un artefacto por hash → citarlo como `artifact://<sha256>` |
| `recall` | re-anclarse tras una compactación, o ponerse al día con otro agente |
| `reflect` | entregar lo que hiciste; COGO propone notas graduadas que vale guardar |
| `lease` | tomar un permiso con TTL antes de una migración, un deploy o una edición masiva |
| `guard` | radiografiar un turno del modelo por presión de manipulación |
| `xray` | radiografiar una respuesta por veracidad |

> **Memoria compartida entre máquinas.** Por HTTP + token, cualquier agente en cualquier
> máquina lee y escribe el mismo vault. `recall` es el cursor que lo convierte de archivo en
> canal: devolvés el cursor que te dio y recibís **solo lo que cambió** — más uno nuevo.

## Dos herramientas que no existen en ningún otro lado

### `gap` — modelar lo que nadie sabe

Toda herramienta de memoria guarda lo que un proyecto sabe. Ninguna guarda lo que **no** sabe.

Sin eso, un agente no puede distinguir un tema que nadie investigó de un tema que no existe.
Las dos ausencias se ven igual: silencio.

```yaml
type: gap
question: ¿El pool de conexiones se satura bajo carga sostenida?
blocks: [migrar-db, subir-replicas]
cost_to_resolve: medio
attempted:
  - se miró el dashboard: no satura en horario normal, nunca se probó con carga real
```

Una brecha **no lleva color**, y ese es todo el punto. Pintarla de roja sería tentador —no hay
evidencia, después de todo— y sería un error: una nota roja *afirma* algo sin respaldo, una
brecha no afirma nada. La convertiría en una mala afirmación en vez de una buena pregunta.

El pack las devuelve **últimas y en su propia sección**, ordenadas por cuántas decisiones traba
cada una. La descripción que ve el agente termina con la línea que importa: *"si estás por
adivinar, registrá la brecha"*.

### `authorize` — la herramienta que puede decir que no

COGO dice cuánto vale cada cosa que sabés. Esa es la mitad. La otra es que **cuánto tiene que
valer depende de para qué**.

| clase de acción | pide por default |
|---|---|
| informativa — responder, explicar | `asserted` |
| reversible — editar, crear, commitear | `check_declared` |
| costosa — deploy, migración, gasto | `claimed_passed` |
| **irreversible** — borrar, publicar, enviar, force push | **`verified`** |

Esa última fila es la línea: **la única clase que exige un check *ejecutado*, no declarado**.
La palabra de un agente no llega ahí.

Y la clase no la elige el agente — porque el que quiere actuar es exactamente quien tiene el
incentivo de clasificarlo bajo. *"Voy a limpiar unos temporales"* puede ser un `rm -rf`. Así
que se decide **dos veces** —lo declarado y lo inferido del texto— y **gana la más estricta**.

```
authorize("limpiar unos temporales con rm -rf en la carpeta de build",
          class: "informative", notes: ["pool-limite-200"])

NOT AUTHORIZED — una acción irreversible necesita respaldo verified, y la nota
no llega
  · pool-limite-200 está en claimed_passed — el check está declarado como pasado
    pero nadie lo ejecutó: corrélo con el runner

action class: irreversible (declarada "informativa", pero el texto dice
              irreversible (borrado de archivos): manda la más estricta)
```

## El visor

- **Vault** — un índice de verdad: búsqueda BM25, filtros buscables, rangos por fecha de
  creación, paginador. Cada nota muestra cuándo nació, cuándo se verificó y cuándo vence.
- **Vigencia** — qué venció o está por vencer. Los hechos tienen fecha de vencimiento.
- **Pack** — el paquete de contexto coloreado, tal cual lo recibe un agente.
- **Grafo** — cómo se conecta tu conocimiento, y cómo el rojo baja por las dependencias.
- **Revisión** — enlaces rotos, notas vencidas y (con un modelo) contradicciones entre notas.
- **Guard** · **Veracidad** — los dos motores, con su evidencia.

Más un editor Markdown que **recomputa el color en vivo mientras escribís**, un **explorador
de GitHub** con un mapa de confianza sobre tu repo, un administrador de instrucciones para
agentes (`AGENTS.md`, `CLAUDE.md`…), auditoría descargable y podable, gestión de múltiples
tokens y exportación del vault en un clic.

### El modo deidad — la sala de guerra

Todo motor de reglas tiene constantes. Repartidas por el código son invisibles: nadie sabe que
están, nadie sabe qué pasa si se mueven, y quien las quiere cambiar tiene que recompilar.

En COGO son **21 en un registro**, cada una con su etiqueta, qué hace, en qué unidad, entre qué
valores es válida y **qué se afloja si se mueve**. El panel se *genera* de ese registro: no hay
una lista de controles escrita a mano que pueda desincronizarse de lo que el motor lee.

Entrar pide escribir **acepto**, y el modal dice sin adornos qué implica: que lo que muevas
re-colorea notas que ya existen sin que nadie las re-verifique, y que cada cambio queda en la
auditoría con tu nombre.

Del otro lado, una segunda pestaña muestra **qué está haciendo el motor ahora mismo** — no
perillas, estado:

- **la integridad de la cadena, primero** — el único dato que invalida a todos los demás
- **los ocho estados del retículo, no tres colores.** La diferencia salta:
  *"17 en `claimed_passed`, 0 en `verified`"* son diecisiete notas que se ven verdes y ninguna
  con un check que alguien haya corrido
- el **registro de eventos**, el **runner**, cada **autorización** que pidió un agente, y la
  **salud del grafo** — ciclos y dependencias colgadas, separados, porque en el Vault se ven
  como el mismo rojo y se arreglan distinto

## Evidencia que se puede volver a chequear

La evidencia no es una sensación: es una referencia que COGO puede ir a verificar de nuevo.

```yaml
evidence:
  - kind: file_read                                  # 9 tipos, de test_result
    ref: worker.go:12                                #  hasta hypothesis y absence
  - kind: command_output
    ref: github://acme/api@main/internal/db.go:88    # anclado al SHA del blob
  - kind: direct_log
    ref: artifact://9f2a…                            # por contenido, inmutable
```

Las referencias de GitHub se anclan al **SHA del blob**. Los artefactos se guardan por su
**SHA-256** —local o en Cloudflare R2— así que `verify` **recomputa** el hash en vez de confiar
en una cita que se pudre. Un **escáner de secretos corre antes de guardar nada**, y se niega
por default.

### Y un archivo que cambió no es una afirmación que cambió

Una nota cita `docker-compose.yml:164`. Alguien corre un formateador, o agrega una línea de
licencia arriba, y la nota se ponía amarilla: cambió el hash del **archivo**. La línea 164
seguía diciendo exactamente lo mismo.

Eso no es un falso positivo inocente. **Un aviso que se dispara por cualquier cosa entrena a
ignorarlo** — y entonces, el día que el archivo cambia de verdad justo donde la nota se
apoyaba, el amarillo ya no significa nada.

Por eso el ancla no es "la línea 164": es el **texto** que estaba en la línea 164, y COGO lo
busca donde haya quedado.

```
sigue igual, donde estaba      →  no importa
sigue igual, en otra línea     →  no importa — y te dice a cuál se movió
solo cambió el espaciado       →  no importa
donde citabas ahora dice       →  IMPORTA
otra cosa
```

Relocalizar tiene una trampa, y está resuelta: una cita de una línea que dice `}` coincide en
cualquier parte. Encontrarla no sería encontrarla, sería adivinar — y adivinar acá significa
**absolver un cambio real**. Así que una región necesita texto distintivo suficiente, y tiene
que aparecer **una sola vez**.

## Cómo se computa el color

```
confianza = meet( eje del check , techo por evidencia , frescura ,
                  contradicciones , materialidad de las citas , el grafo )
```

`meet` es el ínfimo de un retículo: conmutativo, asociativo, idempotente — con tests para las
tres propiedades. La consecuencia práctica es que **ningún eje puede subir un color, solo
bajarlo**. Eso lo vuelve un análisis *must*: afirma solo lo que se sostiene por todos los
caminos.

Una nota es verde solo cuando **nada** la tira abajo. Cada color viaja con su `color_reason`,
así que siempre se puede auditar **por qué**.

### El color es el pliegue de un registro append-only

Desde la versión actual, el color no se computa de los campos de la nota: se computa plegando
un **registro de eventos encadenado por hash** sobre una **máquina de 9 estados**, y resolviendo
después el **mayor punto fijo** sobre el grafo de dependencias.

- **Bitemporal.** Cada evento lleva cuándo pasó en el mundo y cuándo COGO lo registró.
- **A prueba de alteración.** El hash de cada evento incluye el anterior: tocar un evento viejo
  invalida todos los que siguen, y la sala de guerra lo dice arriba de todo.
- **Seguro entre procesos.** Escribir toma un cerrojo del sistema (`flock` / `LockFileEx`): un
  despliegue rodante corre dos contenedores sobre un volumen por unos segundos, y sin el
  cerrojo los dos reclamarían el mismo número de secuencia y partirían la cadena en dos.

La máquina de estados se genera de una sola tabla YAML, y el generador **rompe el build** si
está mal: un estado sin rango, rangos duplicados o con huecos, una transición a un estado
inexistente, una guarda huérfana, un estado inalcanzable, dos transiciones con la misma guarda,
o una decisión sin cubrir.

**Y solo el runner interno puede producir `verified`.** No es una convención: el emisor está
reservado, el journal lo *rechaza* por la puerta común, y una sola función lo emite — así que
un `grep` muestra todos los lugares del sistema capaces de producir una verificación.

### Cinco invariantes, sobre cientos de vaults al azar

Verificadas en cada corrida de la suite, sobre vaults generados con ciclos y dependencias
arbitrarias:

1. **Determinismo** — el orden de iteración de los mapas de Go es deliberadamente aleatorio; el
   resultado no.
2. **La propagación solo baja** — apoyarse en algo no puede volverte más confiable.
3. **Una contradicción nunca mejora nada** — o el sistema premiaría que se oculten.
4. **Quitar evidencia nunca sube** — lo que se afirma se afirma porque hay con qué.
5. **Nadie llega a `verified` sin ejecutar.**

> El plan original pedía TLA+. Un modelo formal es un *artefacto aparte*, y lo que verifica es
> el modelo: nada garantiza que el modelo y el Go digan lo mismo, y en cuanto divergen el
> checker certifica un sistema que no es el que corre. Estas propiedades son más débiles
> —cubren los casos generados, no todos— y son verdaderas **del código que se despliega**.

La invariante 3 encontró un defecto real el día que se escribió: abrir una contradicción sobre
una nota ya refutada la *subía* de `refuted` a `contradicted`. Registrar un problema mejoraba la
nota. Los dos estados son rojos, así que ningún test de color lo hubiera visto.

### Y COGO olvida

Antes calificaba todo lo que entraba y no sacaba nada nunca. El color aísla lo rojo pero no lo
elimina — un vault a tres años son miles de notas, casi todas vencidas, **y lo muerto tapa a lo
vivo**.

Olvidar por antigüedad habría sido el error obvio: la nota más vieja del vault puede ser la que
más se consulta. Así que una nota se vuelve **latente** solo cuando se dan todas: expiró (el
doble de su ventana), nadie depende de ella, y nadie la consultó en 180 días. Las restricciones,
las fijadas a mano, las contradichas y las preguntas abiertas nunca se olvidan.

**Latente no es borrada.** Sigue siendo un archivo, se abre por su id, se ve en el visor con su
motivo — solo deja de entrar en el pack. La búsqueda igual la devuelve, marcada, porque buscar
es cómo se la encuentra para despertarla.

**Y se calcula, no se escribe** — igual que el color. Consultala y deja de estar sin consultar,
así que deja de ser latente. No hay un estado que alguien tenga que acordarse de revertir.

### Y dice quién decidió

Un agente propone Fastify. Decís "dale". El agente registra *"se decidió usar Fastify"*, con su
autor y su evidencia. Mañana lo lee de vuelta como un hecho establecido del proyecto. En cada
vuelta, una opinión se lava en hecho.

Los otros ejes no lo ven: la evidencia puede ser impecable —un `file_read` del `package.json`
que el propio agente escribió— y la procedencia dice quién corrió el check, no quién tuvo la
idea.

Por eso las notas normativas (`decision`, `constraint`) llevan **origen**: `human`, `agent` o
`instrument`. Solo esas dos, porque un `bug` describe cómo es el mundo y ahí la evidencia
responde; una decisión afirma que alguien **eligió**, y ninguna salida de comando puede probar
una elección.

En el pack:

```
- origin: **proposed by an agent** — no human chose this; it is open to revision
```

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
| **Separa "alguien lo dijo" de "lo corrió una máquina"** | — | — | — | ✅ |
| **Guarda lo que el proyecto NO sabe** | — | — | — | ✅ |
| **Puede negarse a una acción por falta de respaldo** | — | — | — | ✅ |
| **Olvida lo que nadie usa** | — | — | — | ✅ |

Las tres que sí son nuestras:

- **Un color que el modelo tiene prohibido escribir.** Donde otras herramientas tienen un
  campo de confianza, suele completarlo el propio LLM — y una vez escrito no cambia nunca
  más. COGO lo computa desde la evidencia, y lo recomputa cada vez que mirás.
- **Duda que se propaga.** Todos los demás resuelven las contradicciones *de a pares* y ahí
  terminan. Un repaso de 435 trabajos sobre memoria de agentes lo nombra como problema
  abierto: *"la supersesión es local; los registros derivados no se vuelven a examinar"*. El
  `meet` sobre la dependencia más débil de COGO es exactamente esa pieza que falta.
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
| [Manual](docs/manual.md) | **el manual completo** — de "qué es esto" hasta el retículo y el punto fijo |
| [Parámetros](docs/parametros.md) | las 21 perillas del modo deidad, una por una |
| [Para agentes de IA](docs/COGO-para-agentes.md) | ponele esto adelante a tu agente |
| [Motor de autonomía](docs/motor-autonomia.md) | Guard, en profundidad |
| [Motor de veracidad](docs/motor-veracidad.md) | xray, en profundidad |
| [Seguridad](docs/seguridad.md) | modelo de amenaza y endurecimiento |
| [Fundamento teórico](docs/fundamento-teorico.md) | por qué la regla de hierro |

## Licencia

MIT — Diego Parrás, CeMIACE / SEUBES / FCE-UBA. Parte de la **Suite Escriba**.
