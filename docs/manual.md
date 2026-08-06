# COGO — el manual

> La memoria de tu proyecto, con un color de confianza que COGO calcula solo y que
> nadie —ni vos, ni un agente— puede escribir a mano.

---

## Cómo leer esto

Está escrito para tres personas distintas y **no hace falta leerlo entero**.

| Si sos… | Empezá en | Y si te enganchás |
|---|---|---|
| **alguien que nunca usó esto** | Partes I y II — el problema y los cinco minutos | la Parte V, el visor |
| **alguien que ya usa agentes de código** | Partes III y IV — las notas y las 16 herramientas | la Parte VII, Guard |
| **alguien que quiere saber si esto es serio** | Parte VI — el retículo, el punto fijo, las invariantes | la Parte IX, lo que no hace |

Un pedido: si sos del tercer grupo, no te saltees la **Parte IX**. Es la que dice
qué no funciona todavía, y es la mitad de por qué se puede confiar en el resto.

---
---

# PARTE I · POR QUÉ EXISTE

## 1. El problema, con un caso concreto

Es martes. Le pedís a tu agente que agregue un endpoint. Lee el repositorio,
encuentra que hay un pool de conexiones, y te dice:

> *"Voy a usar el pool existente. Aguanta 200 conexiones concurrentes."*

Decís "dale". Funciona. Seguís.

**Tres semanas después**, otro agente —u otra sesión del mismo— toma una decisión
de arquitectura apoyándose en que el pool aguanta 200. Nadie lo midió nunca. El
número salió de un comentario en el código, escrito por alguien que ya no está,
sobre una versión del pool que se cambió en abril.

Nada de esto es un error del modelo. El agente hizo bien su trabajo: leyó, dedujo,
propuso. El problema es que **no había forma de distinguir eso de un hecho
medido**, y tres semanas después ya nadie puede saber cuál era cuál.

Eso pasa hoy sin excepción, y tiene tres formas:

**Lo que sabés se pierde.** Termina la sesión y se va. Mañana explicás todo de
nuevo. Un `CLAUDE.md` ayuda, pero lo mantenés a mano y envejece sin avisar.

**Lo que se guarda no dice cuánto vale.** Una nota que dice "el pool aguanta 200"
se lee igual venga de un test que corrió o de una deducción de un modelo a las
tres de la mañana.

**Lo que el agente decidió se lee como lo que vos decidiste.** El agente propone
Fastify, decís "ok", el agente registra *"se decidió usar Fastify"*. Mañana lo lee
como un hecho establecido del proyecto y construye encima. En cada vuelta, una
opinión se lava en hecho.

## 2. Qué hace COGO

COGO guarda **afirmaciones verificables**, no archivos. Cada una con un color que
COGO calcula a partir de su evidencia:

```
🟢 verde     evidencia observada, check pasado, fresca, sin contradicciones
🟡 amarillo  probable: evidencia reportada, o el check no corrió, o venció
🔴 rojo      no confiable: sin evidencia, cita rota o contradicción abierta
```

Y una regla, de la que sale todo lo demás:

> ### Nadie escribe el color. COGO lo computa.
>
> Ni vos, ni un agente, ni editando el archivo a mano. El color es una **función**
> de la evidencia, la frescura, las dependencias y las contradicciones. Si querés
> que algo sea verde no hay forma de pedirlo: hay que darle con qué.

Eso separa a COGO de un archivo de notas con etiquetas. En un archivo de notas,
"importante" es una palabra que alguien escribió. Acá el color es una conclusión.

## 3. Lo que se gana

Un agente conectado a COGO recibe el contexto **ya juzgado**. En vez de leer medio
repositorio para reconstruir qué se decidió y qué se probó, pide un `pack`:

```markdown
# Context pack — "pool de conexiones"
> 3 verified · 1 probable · 1 assumptions · ~420 tokens
> _~420 tokens vs ~3100 leyendo estas notas enteras — 86% menos._

## Supported — evidence holds; check the attestation before acting on it
### pool-limite-200 · architecture
El pool satura a las 200 conexiones concurrentes.
- check: k6 run pool.js → p95 < 400ms · executed

## Probable — likely, not certain
### pool-timeout · command
- caveat: la evidencia es reportada o razonada (tope: amarillo)

## Assumptions — DO NOT RELY
- **pool-recicla-solo**: el pool se recicla solo cada hora
  — _unverified: sin evidencia observada ni reportada_
```

Lo rojo **no viene mezclado con lo verde y rotulado**: viene físicamente separado,
en una sección que dice "no te apoyes en esto". Un agente que lee eso no puede
confundirse sin desobedecer.

---
---

# PARTE II · EN CINCO MINUTOS

## 4. Instalarlo

**Con Docker** (recomendado):

```bash
docker run -d --name cogo -p 8080:8080 \
  -v cogo-vault:/vault \
  -e COGO_MCP_TOKEN=poné-un-token-largo-acá \
  ghcr.io/diegoparras/cogo:latest
```

**Un binario suelto**, sin dependencias — la imagen entera pesa 16 MB porque
adentro no hay nada más que el binario:

```bash
go install github.com/diegoparras/cogo/cmd/cogo@latest
cogo init ~/mi-vault
cogo serve -vault ~/mi-vault -http 127.0.0.1:8080
```

Abrí `http://127.0.0.1:8080`. En loopback no pide token; en cualquier otra
interfaz **se niega a arrancar** sin autenticación (§34).

## 5. Conectar tu agente

COGO habla **MCP**, así que lo entienden Claude Code, Codex, Cursor, Gemini CLI y
cualquier cliente que soporte el protocolo.

Visor → menú → **Conexiones MCP** → *nuevo token* → copiás el bloque y lo pegás en
la configuración de tu cliente:

```json
{ "mcpServers": { "cogo": {
    "type": "http",
    "url": "https://tu-cogo/mcp",
    "headers": { "Authorization": "Bearer cogo_..." } } } }
```

Y `cogo agents` genera el `AGENTS.md` / `CLAUDE.md` que le enseña a tu agente el
protocolo: cuándo pedir contexto, cuándo capturar, y por qué no debe escribir
colores.

## 6. Tu primera nota

Desde el visor: **+ Nueva nota**. Desde un agente: `capture`. Lo mínimo:

```yaml
type: architecture
project: tienda
evidence:
  - kind: command_output
    ref: "k6 run pool.js → p95=312ms (2026-08-04)"
check:
  test: "k6 p95 del pool < 400ms"
```
```markdown
## Claim
El pool satura a las 200 conexiones concurrentes.
```

Mientras escribís, el visor te muestra **el color en vivo** y por qué. Si le sacás
la evidencia se pone roja delante tuyo. Es la forma más rápida de entender la
regla: no se discute con el semáforo, se le da con qué.

---
---

# PARTE III · LA MEMORIA

## 7. Anatomía de una nota

Un archivo Markdown con frontmatter YAML. Nada propietario: se lee con `cat`, se
versiona con git, se edita con cualquier editor.

```yaml
---
id: tienda-checkout-400ms      # estable; si lo omitís se deriva del claim
type: constraint               # §8
project: tienda                # partición: filtrá SIEMPRE por proyecto
evidence:
  - kind: test_result
    ref: "k6 run checkout.js → p95=312ms"
    hash: 9f3a2b…              # línea base para detectar deriva (§24)
    anchor: c41d8f…            # huella de la REGIÓN citada (§24)
check:
  test: "k6 p95 del checkout < 400ms"
  status: passed
  attested: executed           # declarado vs ejecutado (§22)
  attested_by: internal_runner
last_verified: "2026-08-04"
depends_on: [tienda-cache-redis]
origin: human                  # quién lo decidió (§25)
scope: { os: linux }           # dónde vale
pinned: false                  # nunca se olvida (§26)

# ---- computed by COGO · do not edit ----
confidence: green
stale_at: "2027-08-04"
color_reason: "evidencia observada, check pasado, fresca, dependencias verdes"
---

## Claim
El checkout **no puede superar los 400 ms en p95**.
```

Todo lo de arriba de la línea lo escribe una persona o un agente. Todo lo de abajo
lo calcula COGO **en cada lectura**: si editás el `confidence` a mano, se pisa solo.

### Qué hace buena a una nota

- **Un claim que se pueda contradecir.** Si no se puede contradecir no es una
  nota, es una impresión.
- **Evidencia real**: un archivo con su línea, un comando con su salida, un log
  con su hora. No "según recuerdo".
- **El check mínimo**: el test más chico que la probaría. Si no se te ocurre
  ninguno, probablemente sea una opinión.

## 8. Los ocho tipos

| Tipo | Qué guarda | Fresca por |
|---|---|---|
| `constraint` | lo que no puede dejar de cumplirse | 365 días |
| `decision` | qué se resolvió hacer, y por qué | 180 |
| `architecture` | cómo está armado el sistema | 180 |
| `runbook` | cómo se hace tal cosa | 90 |
| `bug` | un error encontrado y su estado | 60 |
| `command` | el comando exacto | 30 |
| `mistake` | un error del que se aprendió | no vence |
| `gap` | **algo que el proyecto NO sabe** | no vence |

Dos merecen párrafo aparte.

**`mistake`** no se gradúa por confianza: no afirma cómo es el mundo, registra algo
que pasó. Ponerle color sería calificar un recuerdo.

**`gap`** es lo único de COGO que ninguna otra herramienta de memoria hace (§14).

## 9. La evidencia y su fuerza

El tipo de evidencia **pone un techo** al color. No importa cuánto se haya
verificado el check: sin evidencia observada no hay verde.

| Fuerza | Tipos | Techo |
|---|---|---|
| **observada** | `direct_log`, `command_output`, `test_result`, `file_read` | verde |
| **reportada** | `doc`, `testimony` | amarillo |
| **razonada** | `inference` | amarillo |
| **ninguna** | `hypothesis`, `absence`, o sin `ref` | rojo |

### Cómo citar, de menos a más fuerte

```
src/checkout.go:42                            ruta local
github://owner/repo@main/src/checkout.go:42   del repositorio  ← preferida
artifact://<sha256>                           artefacto guardado por COGO
```

- **Ruta local**: se resuelve contra la raíz del proyecto. En una instancia
  hosteada no hay copia de trabajo, así que la cita queda sin verificar.
- **`github://`**: COGO baja el archivo por la API, confirma que existe y guarda el
  SHA del blob. Si citás una **rama** y el archivo cambia, la nota **cae sola**
  pidiendo revisión; si citás un **commit fijo**, la cita es inmutable.
- **`artifact://`**: para lo que hoy se pierde — la salida completa del comando que
  falló, el CSV que prueba un conteo. Lo guardás con `stash` y citás la referencia
  que te devuelve. **La clave ES el hash del contenido**, así que la referencia
  misma prueba que nadie lo editó.

> ⚠️ Nunca guardes credenciales en una nota ni en un artefacto. COGO escanea antes
> de escribir y **se niega** si detecta algo que parece una clave: un artefacto
> guardado por su hash no se puede borrar de la historia.

## 10. Los tres colores, en detalle

COGO evalúa de arriba hacia abajo. **La primera cláusula que fuerza un color
gana**, y queda registrada en `color_reason` — por eso cualquier color se puede
discutir.

**Rojo** si: hay una contradicción abierta que la toca · depende de una nota roja ·
no tiene evidencia que cuente · la cita no resuelve · pasó el **doble** de su
ventana de frescura.

**Amarillo** si: cambió lo que la nota citaba · la evidencia es reportada o
razonada · el check no pasó · venció su ventana · depende de una amarilla.

**Verde** solo si no pasa nada de lo anterior.

### El estado de vida es OTRO eje

Archivar una nota **nunca cambia su color**. Son preguntas distintas: el color dice
cuánto la respalda la evidencia, el estado dice si sigue en circulación.

```
activa · archivada · retractada · reemplazada (por `supersedes`)
```

## 11. El pack: lo que realmente ve un agente

`pack` no es "una búsqueda". Es un **digesto presupuestado y ordenado por
confianza**:

1. Toma las notas relevantes (BM25, o por significado si hay embeddings).
2. Las ordena por confianza primero, relevancia después.
3. Corta al presupuesto **de forma monótona**: si se dejó afuera una verde por
   presupuesto, no entra ninguna amarilla. Nunca se gasta el presupuesto en algo
   menos confiable mientras algo más confiable quedó afuera.
4. Renderiza en secciones separadas, con lo rojo aislado.

Y le dice al agente lo que le ahorró.

---
---

# PARTE IV · LAS 16 HERRAMIENTAS

Agrupadas por el momento en que se usan.

## 12. El ciclo normal

**`pack(query, project, token_budget, env)`** — contexto coloreado sobre un tema.
Lo primero, siempre. `env` describe el entorno del agente (`{"os":"linux"}`) para
que COGO marque las notas cuyo alcance no coincide.

**`authorize(action, class?, notes[])`** — *"¿lo que sé alcanza para lo que voy a
hacer?"*. Antes de cualquier acción que cambie algo fuera de la propia respuesta.
Es la herramienta que puede decir que no (§27).

**`capture(type, body, evidence[], check_test, origin, scope, …)`** — guardar un
hallazgo. **Nunca se pasa el color.**

**`verify(id)`** — declarar que el check pasa, con fecha de hoy. Queda registrado
como **declaración**, con la identidad de quien la hizo. Solo el runner interno
produce `executed`.

**`search(query, project)`** — ids, colores y una línea por nota. Nunca cuerpos: la
salida acotada es lo que mantiene barato el contexto.

**`open(id)`** — una nota entera con su color recalculado.

## 13. Arranque y cierre de sesión

**`recall(project, since)`** — re-anclarse. Sin argumentos devuelve **el mandato del
usuario** (sus líneas rojas) y las decisiones verdes vigentes, más un cursor. Se
llama al empezar, **después de una compactación de contexto** —que es cuando se
pierden las restricciones sin que nadie se dé cuenta— y con `since:` para recibir
solo lo que cambió.

**`reflect(summary)`** — al terminar una tarea, un resumen de qué se hizo y
verificó. Si hay un modelo configurado, COGO **propone** notas graduadas que valdría
la pena capturar. Propone: no escribe.

## 14. Decir que no se sabe

**`gap(question, blocks[], cost_to_resolve, attempted[])`**

De todo lo que hay en COGO, esto es lo que ninguna otra herramienta de memoria
hace: **modelar la ausencia de conocimiento**.

Sin esto, un agente no puede distinguir un tema que nadie investigó de un tema que
no existe. Las dos ausencias se ven igual: silencio.

> ### Una brecha no es una nota sin evidencia
>
> Es la distinción que sostiene todo lo demás. Una nota roja **afirma** algo sin
> respaldo. Una brecha **no afirma nada**: declara que hay algo que no se sabe y que
> hay decisiones esperando esa respuesta.
>
> Por eso **no tiene color**. Sería tentador pintarla de roja —no hay evidencia,
> después de todo— y sería un error: la convertiría en una mala afirmación en vez de
> una buena pregunta. Queda fuera del retículo y de la propagación: ni arrastra a
> nadie ni es arrastrada.

```yaml
type: gap
question: ¿El pool se satura bajo carga sostenida?
blocks: [migrar-db, subir-replicas]
cost_to_resolve: medio
attempted:
  - se miró el dashboard: no satura en horario normal, pero nunca se probó con carga real
```

En el pack aparecen **últimas y en su propia sección**:

```
## Open questions — nobody knows this yet; do not guess
### pool-satura
**¿El pool se satura bajo carga sostenida?**
- blocks 2 decision(s): migrar-db, subir-replicas
- cost to resolve: medio
- already tried: se miró el dashboard: no satura en horario normal…
```

El orden de la lista es una heurística del valor de la información: **primero lo que
destraba más decisiones** y, a igualdad, lo más barato de averiguar. Alcanza con
contar; no hace falta un modelo bayesiano.

Y `attempted` es lo que evita que tres personas distintas choquen contra la misma
pared.

La descripción que ve el agente termina con la línea que más importa: *"si estás por
adivinar, registrá la brecha"*.

## 15. Ciclo de vida y coordinación

**`archive(id)`** — guardarla sin borrarla: sale del grafo, del pack y de la
búsqueda, sigue en disco. **Nunca cambia el color.**

**`restore(id)`** — traerla de vuelta.

**`remove(id)`** — borrar de verdad. Solo para basura genuina (proyecto equivocado,
secreto filtrado, duplicado). Deja una lápida en el log.

**`stash(content, kind)`** — guardar un artefacto por su hash y recibir el
`artifact://<sha256>` para citarlo.

**`lease(name, ttl, holder)`** — coordinarse con otros agentes sobre un vault
compartido: un permiso acotado en el tiempo antes de un trabajo riesgoso y no
idempotente (una migración, un deploy). **Expira solo**, así que un agente que se
cuelga no traba el vault para siempre. Es *advisory*, como git: COGO reporta el
conflicto, no lo impide a nivel sistema de archivos. El valor es que la colisión se
vuelve visible.

## 16. Las dos radiografías

**`guard(transcript)`** y **`xray(answer)`** son la otra mitad de COGO y tienen su
propia parte (§30).

---
---

# PARTE V · EL VISOR

## 17. Las pestañas

**Vault** — todas las notas con su color, cuándo se verificaron y cuándo vencen.
Filtros por proyecto, autor y rango de fechas; paginador; click para editar.

Las etiquetas de cada tarjeta son deliberadamente pocas. La regla: una etiqueta que
aparece en el 84% de las tarjetas enseña a **no leer las etiquetas**, así que solo
se muestra lo que varía.

- **`✓ ejecutado`** — el check lo corrió una máquina. Es raro, y es buena noticia.
- **un solo chip de estado**, el más grave: `latente` › `fijada` › `propuesta`. El
  resto está a un hover.

**Contexto** — el pack tal cual lo recibiría un agente. La forma más directa de ver
qué está consumiendo tu asistente.

**Grafo** — las dependencias entre notas con el color propagado. Ahí se ve de un
vistazo si algo verde se apoya en algo rojo.

**Vigencia** — qué está por vencer y qué venció: la cola de re-verificación.

**Revisión** — contradicciones detectadas (necesita un modelo configurado).

**Guard / Xray** — las dos radiografías.

## 18. El editor

El color se recalcula **en vivo** con su razón. Cada cita muestra si resuelve:

```
✓ resuelve       el archivo citado existe
✓ sigue igual    el archivo cambió, pero no en lo que esta nota cita
⟳ cambió         cambió justo lo que la nota cita → baja a amarillo
✗ no resuelve    el archivo no existe → esta evidencia NO cuenta
— sin chequear   COGO no puede verificarlo sin conexión
```

Los campos cambian según el tipo:

- **`decision` / `constraint`** muestran *Quién lo decidió* (§25).
- **`gap`** muestra la pregunta, qué traba, cuánto cuesta y qué se intentó — y
  **evidencia y check desaparecen**, porque una brecha no tiene qué respaldar.

## 19. Qué archivos escribe COGO

Todo vive en el vault, en texto plano:

```
mi-vault/
  *.md                     las notas
  log.md                   qué hizo cada agente, en orden
  .cogo/
    journal/YYYY-MM.jsonl  el registro de eventos (§20)
    uso.json               qué notas se consultan (§25)
    parametros.json        solo lo que difiere del default (§31)
    tokens.json            los tokens MCP, hasheados
    audit.jsonl            quién llamó a qué
    history/<id>.jsonl     cómo fue cambiando el color de cada nota
    contradictions.json    las contradicciones abiertas
    runner.yaml            los checks que este vault autoriza (§27)
    trash/                 lo borrado, por si acaso
```

Sin base de datos, sin demonio, sin cuenta. `git init` en el vault y tenés
historial completo.

---
---

# PARTE VI · CÓMO DECIDE COGO

> La parte para quien quiere saber si esto es serio.

## 20. El color es un *meet*, no una fórmula

COGO no promedia. El color sale de combinar ejes independientes tomando **siempre
el más débil**:

```
color = meet( eje del check
            , techo por evidencia
            , frescura
            , contradicciones
            , materialidad de las citas
            , calibración del emisor      ← apagado por default
            , lo que llega por el grafo )
```

`meet` es el ínfimo de un retículo, y tiene tres propiedades que hacen que esto
funcione: es **conmutativo** (el orden de evaluación no importa), **asociativo**
(agrupar distinto da lo mismo) e **idempotente**. Hay tests para las tres.

La consecuencia práctica: **ningún eje puede subir un color**. Solo bajarlo. Un
motor así se llama de análisis *must* — afirma solo lo que puede sostener por todos
los caminos.

## 21. El registro de eventos: el recibo

El color **no se calcula del estado de la nota**: se calcula plegando un registro.

```json
{"seq":41,"valid_time":"2026-08-04T19:22:01Z","tx_time":"2026-08-04T19:22:01Z",
 "note_id":"pool-limite-200","kind":"CheckExecuted","emitter":"internal_runner",
 "guard":"ejecucion_ok","payload":{"check":"k6-pool","exit_code":0},"prev":"a41f…"}
```

Tres propiedades salen de ahí:

**Es bitemporal.** Cada evento tiene *cuándo pasó en el mundo* (`valid_time`) y
*cuándo COGO lo registró* (`tx_time`). Son distintos cuando algo se supo después, y
tenerlos separados permite preguntar "¿qué sabíamos el 3 de agosto?" sin mentir.

**Es append-only y encadenado.** El hash de cada evento incluye el del anterior:
**alterar un evento viejo invalida todos los que siguen**, y `Verificar()` lo
detecta. La sala de guerra lo muestra arriba de todo porque es el único dato que
invalida a todos los demás.

**Y dos procesos no se pisan.** Escribir toma un cerrojo del sistema operativo
(`flock` / `LockFileEx`) y, con él en la mano, relee la punta del registro. Sin eso,
un despliegue rodante —que levanta el contenedor nuevo antes de bajar el viejo—
produce dos eventos con el mismo número y dos ramas de la cadena.

## 22. La máquina de estados

Ocho estados estables, de menos a más confiable, más uno transitorio:

```
quarantined < refuted < contradicted < stale < asserted
            < check_declared < claimed_passed < verified

verifying — transitorio, fuera del retículo
```

`internal/confidence/transitions.yaml` es la **fuente única**: 9 estados, 10
eventos, 16 transiciones. El Go sale de ahí, y el generador **rompe el build** si la
tabla está mal: un estado sin rango, rangos duplicados o con huecos, una transición
a un estado inexistente, una guarda huérfana, un estado inalcanzable, un transitorio
con rango, dos transiciones con la misma guarda, o una decisión sin cubrir.

Dos cosas la separan de una máquina de estados común:

**Las guardas son nombres, no expresiones.** Se agrupan en `decisions`, y los
miembros de una decisión son mutuamente excluyentes **por construcción** — no porque
alguien se acordó de escribir el `else`.

**Los eventos negativos son techo, no salto.** Cinco están marcados `degrada: true`
y el fold los aplica con `meet` en vez de transicionar. Salió de un defecto real:
abrir una contradicción sobre una nota ya refutada la **subía** de `refuted` a
`contradicted`. Registrar un problema mejoraba la nota. Los dos estados son rojos,
así que ningún test de color lo hubiera visto nunca.

### La línea que importa

**Solo el runner interno puede producir `verified`.** No es una convención: el
emisor `internal_runner` está reservado, el journal **rechaza** cualquier intento de
escribirlo por la puerta común, y hay una sola función que lo emite —
`AppendEjecucion`— así que un `grep` muestra todos los lugares del sistema capaces
de producir una verificación.

Todo lo demás, por más verde que se vea, es una **declaración**.

## 22b. El sello: probar que es el mismo registro

La cadena de hashes prueba que el registro es **internamente consistente**. No
prueba nada contra quien tiene el archivo.

> Quien es dueño del vault puede regenerar el registro entero desde cero —eventos
> nuevos, hashes recalculados, cadena perfecta— y no hay forma de notarlo, porque
> no existe ningún punto de referencia fuera de su disco.

Alcanza con publicar **un hash** en algún lugar que no controles: la cabeza de la
cadena, que resume toda la historia anterior. Si el registro se reescribe, la
cabeza que sale de los eventos de hoy no coincide con la que se publicó entonces.

```bash
cogo sellar -nota "commit a1b2c3 del repo de actas"

  COGO · sello del registro
    evento:  45
    cabeza:  1fe2d4f6b5dbefcf072ec84c500fc17ca0fce05e5bc1f01b3d46b33bd9706d1e
    cuando:  2026-08-06T11:30:43Z

  Publicá ESTAS TRES LÍNEAS en algún lugar que no puedas reescribir solo.
```

Y después, en cualquier momento:

```bash
cogo sellos

  MAL  evento 45   NO COINCIDE — el evento 45 sellado el 2026-08-06 tenía la
                   cabeza 1fe2d4f6b5db… y hoy da 582d5938af7f…. El registro se
                   reescribió después de publicar este sello
```

**Dos destinos, y ninguno automático por default.** `manual` te da el sello y lo
publicás vos donde quieras — un commit firmado, un mail, un mensaje con fecha.
`https` lo manda a una URL que le des. COGO no trae un destino "que anda solo":
eso sería mandar la cabeza de tu registro al servidor de un tercero sin que lo
hayas pedido.

**Un sello que COGO se manda a sí mismo no prueba nada contra COGO.** Por eso el
destino manual no publica: imprime y te dice que lo publiques. Y el archivo local
guarda siempre **dónde** fue, porque un hash guardado al lado del registro que
resume no vale nada.

### Qué prueba y qué no

Prueba que en el momento en que se publicó, el registro era exactamente el que
produce esa cabeza. Si el lugar donde lo publicaste tiene fecha propia, prueba
además **cuándo**.

**No prueba que lo que dicen los eventos sea cierto.** Un sello es sobre la
historia, no sobre los hechos.

### Por qué no es una blockchain

COGO ya tiene la parte útil: una cadena de bloques enlazados por hash. Lo que una
blockchain agrega encima es **consenso distribuido**, que resuelve el problema de
ordenar escrituras entre partes que desconfían entre sí.

COGO no tiene esa forma: hay **un escritor por vault**, forzado por un cerrojo del
sistema operativo (§21). Comprar consenso para un solo escritor es pagar por un
problema que no se tiene — y obligaría a que COGO dependa de una red, cuando hoy
todo lo que la toca es un accesorio opcional.

Y anclaría lo equivocado: las notas **deben** ser editables y el color **debe**
recalcularse. Lo inmutable es el registro de lo que pasó, y ya lo es.

## 23. El punto fijo sobre el grafo

Las notas dependen unas de otras, y esas dependencias pueden tener ciclos. "¿Qué
color le corresponde a cada una?" no tiene respuesta obvia cuando A depende de B y B
de A.

COGO lo resuelve con el **mayor punto fijo**: arranca suponiendo que todo está
verificado y baja por una lista de trabajo hasta que nada más cambia.

Knaster-Tarski garantiza que existen el mayor y el menor. **Elegir el mayor es una
decisión semántica**, no un detalle: un ciclo de notas sanas se queda sano —nada
externo lo empuja hacia abajo— mientras que con el menor punto fijo el mismo ciclo
colapsaría a rojo. Hay un test que lo demuestra sobre el mismo grafo: lfp →
`quarantined`, gfp → `verified`.

## 24. La materialidad de las citas

Una nota cita `docker-compose.yml:164`. Alguien corre un formateador, o agrega una
línea de licencia arriba, y la nota se ponía amarilla: cambió el hash del
**archivo**. La línea 164 seguía diciendo exactamente lo mismo.

Eso no es un falso positivo inocente. **Un aviso que se dispara por cualquier cosa
entrena a la gente a ignorarlo**, y entonces, el día que el archivo cambia de verdad
justo donde la nota se apoyaba, el amarillo ya no significa nada.

Por eso el ancla no es "la línea 164": es el **texto** que estaba en la línea 164, y
COGO lo busca donde haya quedado.

| | |
|---|---|
| sigue igual, donde estaba | no importa |
| sigue igual, **en otra línea** | no importa — y dice a cuál se movió |
| difiere solo en espaciado | no importa |
| **donde citabas ahora dice otra cosa** | **importa** |
| **lo citado no está en ninguna parte** | **importa** |

Normalizar espaciado absorbe `gofmt`, `prettier` y el cambio de tabulaciones.
Ignorar comentarios **no** sería seguro —un comentario puede decir "esto está
roto"— así que se normaliza el espaciado y nada más. Y la sangría se conserva como
hecho aunque no como ancho: dos a cuatro espacios es cosmético, sacarle la sangría a
un bloque de Python no lo es.

**Lo que no se absuelve.** Relocalizar tiene una trampa: una cita de una línea que
dice `}` coincide en cualquier parte del archivo. Encontrarla no sería encontrarla,
sería adivinar — y adivinar acá significa **absolver un cambio real**. Así que una
región necesita texto propio suficiente para ser reconocible, y tiene que aparecer
**una sola vez**. De un empate no se concluye nada.

## 25. El origen: quién decidió

Un agente propone Fastify. Decís "dale". El agente captura *"se decidió usar
Fastify"*, con su autor y su evidencia. Mañana lo lee de vuelta como un hecho
establecido del proyecto.

Los ejes anteriores no lo ven: la evidencia puede ser impecable —un `file_read` del
`package.json` que el propio agente escribió— y la procedencia dice quién corrió el
check, no quién tuvo la idea.

Por eso las notas **normativas** llevan origen:

```
origin: human       lo decidió una persona
origin: agent       lo propuso el agente
origin: instrument  salió de un instrumento: nadie lo eligió, se midió
```

Solo `decision` y `constraint`. Un `bug` o un `runbook` describen cómo es el mundo,
y ahí la evidencia responde; una decisión afirma que alguien **eligió**, y **ninguna
salida de comando puede probar una elección**.

**No baja el color, y es deliberado.** Un techo obligaría a ratificar a mano cada
decisión que tome un agente, y COGO se juega en no agregar tareas: una herramienta
que pide trabajo para seguir siendo confiable termina no usándose. La etiqueta da
casi todo el valor — el que lee sabe que eso se puede revisar.

En el pack:

```
- origin: **proposed by an agent** — no human chose this; it is open to revision
```

## 26. El olvido

COGO calificaba todo lo que entraba y no sacaba nada nunca. El color aísla lo rojo
pero no lo elimina: sigue existiendo, sigue costando, sigue apareciendo. Un vault a
tres años son miles de notas, casi todas vencidas — **y lo muerto tapa a lo vivo**.

Olvidar por antigüedad habría sido el error obvio: la nota más vieja del vault puede
ser la que más se consulta. COGO puede hacerlo mejor porque sabe cosas que un
contador de fechas no. Una nota se vuelve **latente** cuando se dan **todas**:

| | Por qué |
|---|---|
| expiró | pasó el **doble** de su ventana; una apenas vencida todavía se re-verifica |
| nadie depende de ella | si algo se apoya, sacarla lo dejaría en el aire |
| nadie la consultó en 180 días | la condición que hace que esto no sea la edad |

Y **nunca** salen: las restricciones (sostienen todo lo demás), las fijadas a mano,
las que tienen una contradicción abierta (esconderla escondería el conflicto) y las
preguntas abiertas.

**Latente no es borrada.** Sigue en el vault, sigue siendo un archivo, se abre por su
id y se ve en el visor con su motivo. Lo que cambia es que deja de entrar en el pack.
La búsqueda **sí** la devuelve, marcada — buscar es cómo se la encuentra para
despertarla, y una nota que no se puede encontrar no está olvidada, está perdida.

**Y se calcula, no se escribe** — igual que el color. Nadie marca una nota como
latente: la condición se evalúa cada vez. Por eso despertar no tiene ceremonia:
consultala y deja de estar sin consultar, así que deja de serlo.

> Para que esto no fuera adivinar, COGO registra **qué notas se consultan**: las que
> entran en un pack y las que se abren por id. Aparecer en una búsqueda no cuenta —
> eso mediría coincidencias léxicas, no uso. El registro guarda su propia fecha de
> inicio, así que instalar esta versión **no vuelve latente a medio vault el primer
> día**: una nota sin registro no es una que nadie consultó, es una que nadie
> consultó *desde que se empezó a mirar*.

## 27. Cuánto respaldo pide cada acción

Hasta acá COGO dice cuánto vale cada cosa que sabe. Esa es la mitad: la otra es que
**cuánto tiene que valer depende de para qué**.

Explicar algo apoyado en una nota amarilla es aceptable. Borrar una base de datos
apoyado en la misma nota no lo es. Un solo umbral para las dos cosas pide de más en
un lado o de menos en el otro — y en la práctica termina siendo de menos, porque los
umbrales bajan hasta donde no molestan.

| Clase | Qué es | Pide por default |
|---|---|---|
| informativa | responder, explicar, resumir | `asserted` |
| reversible | editar, crear, commitear | `check_declared` |
| costosa | deploy, migración, gasto | `claimed_passed` |
| **irreversible** | borrar, publicar, enviar, force push | **`verified`** |

La última es la línea: **la única clase que exige un check ejecutado y no
declarado**. Ahí la palabra de un agente no alcanza, y todo el aparato anterior —el
runner, la procedencia, el retículo— existe para poder trazarla.

### Y la clase no la decide el agente

Porque el que quiere hacer algo es exactamente quien tiene el incentivo de
clasificarlo bajo. *"Voy a limpiar unos temporales"* puede ser un `rm -rf`.

Así que la clase se decide **dos veces** —lo que el agente declara y lo que se
infiere del texto— y **gana la más estricta**. Un agente puede subir la exigencia
sobre sí mismo; no puede bajarla.

```
authorize("limpiar unos temporales con rm -rf en la carpeta de build",
          class: "informative", notes: ["pool-limite-200"])

NOT AUTHORIZED — una acción irreversible necesita respaldo verified, y la nota
no llega
  · pool-limite-200 está en claimed_passed — el check está declarado como pasado
    pero nadie lo ejecutó: corrélo con el runner

action class: irreversible (declarada "informativa", pero el texto dice
              irreversible (borrado de archivos): manda la más estricta)

Do not proceed. Either raise the support to the required level, or tell the human
what is missing and let them decide. Reporting the block is a valid outcome;
working around it is not.
```

Y **una sola nota floja hunde el pedido**: te apoyás en el conjunto, y el conjunto
vale lo que su parte más débil.

## 28. El runner, y su modelo de amenaza

Ejecutar comandos que salen de notas escritas por un LLM es la superficie de ataque
más grande que COGO puede tener. Y **una lista de comandos permitidos no la acota**:
si el agente escribe la nota *y* el comando, y `go test` está permitido, entonces el
agente hace que `go test` corra código que él mismo escribió — porque `go test`
ejecuta `TestMain` e `init()`. Lo mismo con `npm test`, que corre lo que diga el
`package.json`.

Por eso **el comando no sale de la nota**. Los declara una persona en
`.cogo/runner.yaml`, con su directorio y su timeout; la nota solo **elige** cuál de
esos checks le aplica, por nombre.

```yaml
enabled: false          # viene apagado: un COGO recién instalado no ejecuta nada
checks:
  - id: go-test
    command: ["go", "test", "./..."]   # argv, no una línea de shell: sin shell no
    workdir: /ruta/absoluta/al/repo    # hay expansión, ni tuberías, ni ";" para
    timeout: 2m                        # encadenar otra cosa
```

Un agente puede pedir que se corra `go-test`. **No puede inventar qué se ejecuta.**

## 29. Las invariantes

Cinco propiedades del motor, verificadas sobre **cientos de vaults generados al
azar** —con ciclos, dependencias arbitrarias y evidencia mezclada— en cada ejecución
de la suite:

1. **Determinismo.** El mismo vault da el mismo resultado siempre. No es trivial: el
   motor recorre mapas de Go, cuyo orden de iteración es deliberadamente aleatorio.
2. **La propagación solo baja.** Apoyarse en algo no puede volverte más confiable de
   lo que sos.
3. **Una contradicción nunca mejora.** Registrar un problema no puede subirle el
   estado a nadie, o el sistema premiaría que se oculten.
4. **Quitar evidencia nunca sube.** Lo que se afirma se afirma porque hay con qué.
5. **Nadie llega a `verified` sin haber ejecutado.**

> El plan original pedía modelar esto en TLA+. Un modelo formal es un **artefacto
> aparte**, y lo que verifica es el modelo: nada garantiza que el modelo y el Go
> digan lo mismo, y en cuanto divergen —lo hacen siempre— el model checker pasa a
> certificar un sistema que no es el que corre. Estas propiedades son más débiles
> (cubren los casos que se generan, no todos) y son verdaderas **del código que se
> despliega**. Para un motor que cabe en un proceso, es el intercambio correcto.

La invariante 3 encontró un defecto real el día que se escribió: el de la
contradicción que mejoraba una nota (§22).

### Y el corte se mide, no se anuncia

Cada cambio del motor se corre contra el vault real **antes** de aplicarlo, y el
test falla si alguna nota cambia de color sin que alguien lo haya declarado y
explicado. Las cuatro veces que se cambió el motor: **0 de 25 notas cambiaron**.

---
---

# PARTE VII · LA OTRA MITAD: GUARD Y XRAY

Hasta acá COGO responde *"¿cuánto vale lo que sé?"*. Guard y Xray responden otra
pregunta, sobre el mismo turno de conversación: **"¿esto que me está diciendo el
modelo, me está empujando?"** y **"¿lo que afirma lo puede sostener?"**.

Son la parte de COGO que mira **hacia el otro lado**: no a la memoria, sino al
diálogo.

## 30. Xray — la radiografía de veracidad

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

## 31. Guard — la radiografía de manipulación

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
---

# PARTE VIII · OPERARLO

## 32. El modo deidad

Todo motor de reglas tiene constantes. Cuántos días dura fresca una decisión,
cuánta evidencia hace falta para autorizar un borrado, cuántas observaciones antes
de creerle a una estadística. Repartidas por el código son invisibles: nadie sabe
que están, nadie sabe qué pasa si se mueven, y quien las quiere cambiar tiene que
recompilar.

En COGO están las **21 en un registro**, cada una con su etiqueta, qué hace, en qué
unidad, entre qué valores es válida y **qué se afloja si se mueve**. El panel del
visor se **genera de ahí**: no hay una lista de controles escrita a mano que pueda
desincronizarse de lo que el motor lee.

### El portón

Entrar pide escribir **acepto**, y el modal dice qué implica sin adornos: que lo
que muevas cambia notas que **ya existen** sin que nadie las re-verifique, que
algunos parámetros dejan que una acción irreversible se autorice con la palabra de
un agente, que todo queda en la auditoría con nombre, y que los defaults son los
que están probados.

Se pregunta una vez por sesión del navegador. No es un susto decorativo: es lo
único que separa entrar a propósito de entrar por curiosidad.

### La sala de guerra

Dos pestañas. **Controles** son las 21 perillas. **Estado** es lo que el motor está
haciendo ahora mismo:

- **La cadena, primero.** Íntegra o rota. Va arriba de todo porque es el único dato
  que invalida a todos los demás.
- **Los ocho estados, no los tres colores.** El Vault muestra verde/amarillo/rojo
  porque es lo que hace falta de un vistazo; acá está la verdad completa. Y la
  diferencia salta: *"17 en `claimed_passed`, 0 en `verified`"* significa diecisiete
  notas que se ven verdes y ninguna con un check que alguien haya ejecutado.
- **El registro** — los últimos 60 eventos con su nota, tipo, emisor y guarda.
- **El runner** — qué checks están autorizados y cuándo corrió cada uno.
- **Las autorizaciones** — qué pidió cada agente, qué se le permitió y en qué se
  apoyaba. Toda consulta queda, autorice o no: lo que se quiere poder reconstruir
  es en qué se apoyó cada acción, sobre todo las que pasaron.
- **Salud del grafo** — ciclos y dependencias colgadas, **separados**: en el Vault
  las dos se ven como un rojo cualquiera y se arreglan distinto.

### Qué se guarda

Solo lo que difiere del default. **Un vault que nadie configuró no tiene siquiera
archivo de parámetros**, y actualizar COGO mueve los defaults hacia adelante sin
pisar lo que alguien decidió a mano. Es la misma razón por la que un `.gitconfig`
no lista las 400 opciones de git.

El detalle de los 21 está en [`parametros.md`](parametros.md).

## 33. Desplegarlo

La guía completa está en [`deploy.md`](deploy.md). Lo esencial:

**Una instancia por vault.** Es lo que hace EasyPanel por default con una app de
una réplica. El caso que no es obvio: un **despliegue rodante** levanta el
contenedor nuevo antes de bajar el viejo, y por unos segundos hay dos COGO con el
mismo volumen. Eso está cubierto por el cerrojo (§21).

Lo que **no** está cubierto es dos máquinas distintas contra un NFS compartido: ahí
los cerrojos de red no son confiables. Si necesitás varias instancias, dales un
vault a cada una y federalas.

**El volumen es obligatorio.** Sin `/vault` montado, cada actualización te borra
las notas.

**Y actualizar es seguro:** los campos nuevos son `omitempty`, sin archivo de
parámetros manda el default, y como el registro de uso arranca el día que se
instala, **nada llega al umbral de olvido por meses**.

## 34. Seguridad

**Se niega a arrancar** en una interfaz pública sin autenticación. Si sabés que el
puerto ya es privado (firewall, túnel SSH, VPN), lo forzás con
`COGO_ALLOW_INSECURE=1`.

**Los tokens se guardan hasheados.** El valor completo se muestra una sola vez, al
crearlo.

**Escaneo de secretos antes de escribir.** COGO se niega a guardar una nota o un
artefacto si detecta algo que parece una credencial. Con un artefacto la razón es
más fuerte: guardado por su hash, no se puede borrar de la historia.

**Auditoría.** `.cogo/audit.jsonl` registra quién llamó a qué herramienta y cuándo,
con auto-recorte configurable.

**Login federado opcional** vía Lockatus (OIDC), para un equipo.

Los detalles están en [`seguridad.md`](seguridad.md).

---
---

# PARTE IX · LO QUE NO HACE

La parte más importante para decidir si te sirve.

## 35. Lo que está apagado a propósito

**La calibración por emisor.** Cuando alguien declara "el check pasa" y después el
check ejecutado falla, eso queda registrado. Con suficientes casos se podría dejar
de creerle igual a todos. Está implementada, desplegada y midiendo — **y apagada**,
porque necesita meses de ejecuciones reales. Hasta entonces cualquier conclusión
sería ruido con forma de estadística.

**Las ventanas por supervivencia.** Los 180 días de `decision` no salen de ningún
lado: son una intuición razonable. El vault tiene la respuesta —cada nota
desmentida dice cuánto duró siendo cierta— y hay un estimador de Kaplan-Meier
esperando datos. También apagado.

Las dos se ven en la sala de guerra: **qué dirían si las encendieras, y cuánto les
falta**. Un módulo apagado e invisible es un módulo que no existe.

**El runner.** Viene apagado. Un COGO recién instalado no ejecuta nada.

## 36. Los límites conocidos

**El origen es una etiqueta, no un freno.** Marca que una decisión la propuso un
agente; no impide que se use.

**Todo se midió contra un vault de 25 notas.** Los números de escala —el olvido, el
caché del registro— salen de benchmarks sintéticos, no de un vault grande de
verdad. Nadie corrió COGO con 4.000 notas.

**El registro no se sirve por HTTP.** Se ve en la sala de guerra (los últimos 60
eventos) y está entero en disco, pero no hay API para consultarlo ni para hacer
preguntas bitemporales del tipo "¿qué sabíamos el 3 de agosto?". La capacidad está
en el modelo de datos; la cara, no.

**La detección de contradicciones necesita un modelo.** Sin uno configurado, COGO
funciona igual — solo que sin esa detección.

**Guard depende de que declares tu mandato.** Sin él degrada a modo informativo, y
esa degradación es correcta: sin saber qué no estás dispuesto a hacer, manipulación
y persuasión legítima son indistinguibles.

**La materialidad no aplica a `github://`.** La API devuelve el SHA del blob, no el
contenido, y sin contenido no hay región que comparar. Esas citas siguen con la
regla anterior: si el archivo cambió, la nota baja.

## 37. Lo que COGO no es

**No es una base de conocimiento.** No metas los archivos del proyecto adentro: el
repositorio ya los versiona con historia y `diff`. En COGO va lo que *no* está en
los archivos o costaría re-derivar — por qué se decidió algo, qué restricción rige,
qué ya se probó y falló.

**No es un RAG.** No trocea documentos ni los recupera por similitud. Guarda
afirmaciones con evidencia, y las devuelve juzgadas.

**No es un sistema de permisos.** `authorize` responde una pregunta; no puede
impedir que un agente actúe igual. Como los leases: hace visible el conflicto, no
lo bloquea a nivel sistema operativo.

**No es un juez de verdad.** COGO nunca dice "esto es verdad". Dice cuánto lo
respalda lo que se declaró, y quién lo comprobó.

---

## Y una última cosa

Todo lo que dice este manual sobre el motor **está verificado por tests que corren
en cada commit**: 28 paquetes, las cinco invariantes sobre cientos de vaults
generados al azar, y el corte medido contra un vault real antes de cada cambio.

Si algo de acá resulta falso, es un bug — y hay dónde reportarlo.

---

**Documentos hermanos**

- [`COGO-para-agentes.md`](COGO-para-agentes.md) — la guía que lee tu agente
- [`parametros.md`](parametros.md) — los 21 parámetros, uno por uno
- [`deploy.md`](deploy.md) — desplegarlo en serio
- [`instalacion.md`](instalacion.md) — instalación paso a paso
- [`seguridad.md`](seguridad.md) — el modelo de seguridad
- [`motor-autonomia.md`](motor-autonomia.md) · [`motor-veracidad.md`](motor-veracidad.md) — Guard y Xray en profundidad
- [`fundamento-teorico.md`](fundamento-teorico.md) — por qué el "no sé" es estructuralmente externo
