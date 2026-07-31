# COGO — notas para retomar en su propio proyecto

Salió de una conversación sobre otra cosa (la plataforma de Talento IT Patagonia),
así que esto es el extracto limpio. Autocontenido: se puede leer sin ese contexto.

30 de julio de 2026.

---

## De dónde salió

Estaba buscando dónde guardar tres documentos largos de otro proyecto y aparecieron
dos ideas que merecen su propio lugar:

1. Que COGO guarde artefactos en un R2 de Cloudflare.
2. Que COGO sea la memoria compartida entre instancias de agentes en máquinas
   distintas —y entre herramientas distintas—.

La segunda derivó en una revisión de qué existe hoy en el mercado.

---

## Punto de partida: qué hay hecho

- COGO expone **MCP sobre HTTP** en `https://cogo.go.websiteonline.org/mcp`, con
  autenticación por bearer.
- Herramientas que publica: `capture · recall · search · pack · open · guard ·
  verify · xray · reflect · archive · remove · restore`.
- Hay además un **COGO local** distinto: un `cogo.exe serve` contra un vault en
  disco, en `E:\Claude\Escriba-Suite\cogo\vault`. Son dos vaults separados.
- En Claude Code quedó registrado el hosted en alcance global como **`cogo-nube`**,
  a propósito con otro nombre: con el mismo nombre para los dos, cuál te toca
  dependería del directorio desde el que abrís la sesión, y escribir una nota en el
  vault equivocado es un error caro y silencioso.

### La forma de `capture`, que define el producto

```
capture(type, body, evidence[], check_test, project, depends_on, supersedes, caused_by, id)
```

- `type`: `decision · bug · runbook · architecture · constraint · command · mistake`
- `body`: markdown con una afirmación, refutación opcional y una comprobación mínima
- `evidence[]`: `{kind, ref}` con `kind` en
  `direct_log · command_output · test_result · file_read · doc · testimony ·
  inference · hypothesis · absence`
- **el color no se setea: lo computa COGO**

Eso deja claro que COGO **no es un depósito de documentos**: su unidad es una
afirmación verificable. Un documento de 23.000 caracteres metido como una nota
sería una afirmación del tipo "este texto existe", trivialmente cierta e imposible
de puntuar. La maquinaria de frescura y veracidad se queda sin nada que calificar.

---

## Idea 1 — Artefactos en R2

### Lo que R2 no arregla

No habilita "documentos largos". Si el documento va a R2 y la nota dice "está en
`r2://…`", seguís teniendo una nota sin afirmación verificable. Tendrías un
servidor de archivos con una nota colgada.

### Lo que sí arregla, y es grande

Hoy `evidence.ref` apunta a artefactos que viven afuera —commit y línea, marca de
tiempo de un log, salida de un comando— y todos se pudren: el repo se reescribe,
el log rota, la sesión se cierra. Cuando el artefacto desaparece, la nota sigue
declarándose verde y ya nadie puede comprobarlo.

Con R2, COGO **guarda el artefacto**, no la referencia al artefacto.

### El golazo no es el almacenamiento: es el direccionamiento por contenido

Si la clave en R2 es el **SHA-256 del contenido**, la referencia demuestra por sí
sola que el artefacto no se editó. Ahí `verify` deja de ser declarativo y pasa a
computar: baja el objeto, lo hashea, compara. La veracidad deja de ser una
propiedad que alguien afirmó y pasa a ser una que se recalcula.

De regalo: deduplicación. El mismo log capturado por tres agentes ocupa una vez.

### Cuatro cosas que rompen si se dejan para después

**Secretos.** Las salidas de comandos y los logs son exactamente donde se filtran
credenciales, y con hash inmutable quedan para siempre. Caso real de la
conversación: el panel de despliegue de ese proyecto imprime en su log la
contraseña de Postgres, dos claves de R2, la de Resend y la de Anthropic. Volcar
ese log crudo habría metido las cinco en el vault. **Guard tiene que correr antes
de subir, no después.**

**El derecho a borrar.** Con evidencia inmutable y por contenido, borrar una nota
ya no borra la evidencia. La Papelera tiene que llegar hasta R2, y con
deduplicación hay que contar referencias antes de borrar un objeto que quizá
sostiene otra nota.

**La paridad con el vault local.** Si el hosted guarda en R2 y el local en disco, se
bifurca el producto. Una sola interfaz de almacenamiento con dos backends —disco y
S3— sale casi gratis, porque R2 habla S3.

**El tamaño.** Sin tope y sin una pregunta de "por qué vale la pena conservar esto",
el vault se vuelve un cajón de sastre y la búsqueda semántica empeora con cada
archivo basura.

### Qué mandar a R2 y qué no

- **Sí:** la salida completa del comando que falló, el PDF que mandó el cliente, la
  captura de la pantalla rota, el CSV que prueba un conteo. Eso es evidencia y hoy
  se pierde.
- **No:** documentación larga. Va al repositorio, que es donde se versiona y se lee
  con `git blame`.

---

## Idea 2 — Memoria compartida entre máquinas

### Ya funciona

En el momento en que COGO expone MCP sobre HTTP con token, cualquier agente en
cualquier máquina con esa config lee y escribe el mismo vault. No hay nada que
construir para que "se comuniquen".

### Lo que falta es coordinación, que es otro problema

**Un cursor.** `recall` devuelve la memoria que sostiene el proyecto, pero no hay
forma de preguntar *qué cambió desde la última vez que miré*. Sin eso, la máquina B
no se entera de lo que hizo la A salvo releyendo todo. Un `recall(desde: <marca>)`
que devuelva solo lo nuevo es probablemente veinte líneas y es **lo que más
desbloquea de toda la lista**: convierte el vault de archivo compartido en canal.

**Identidad.** Con un solo bearer compartido, el vault no distingue quién escribió
qué. Para memoria da igual; para agentes que se coordinan es central. Tokens por
agente, y la pantalla de Auditoría MCP ya tiene dónde mostrarlo.

**El alcance de validez de cada afirmación.** La más invisible y la que más caro
sale. Una nota capturada en Windows dice "el build falla con `npm ci`". Es verdad
*ahí*; en Linux es falsa, y sin embargo llega verde a la otra máquina. La memoria
compartida entre máquinas necesita registrar bajo qué condiciones se sostuvo la
afirmación —sistema operativo, commit, versión de runtime—. `evidence.ref` con
commit y línea hace parte del trabajo; le falta el entorno.

**Leases.** Si la máquina A está corriendo una migración y la B arranca la misma, el
vault no lo impide. Versión barata: una nota `constraint` con vencimiento. Versión
seria: un endpoint que otorgue y renueve un permiso.

### El riesgo nuevo, que es el reverso del beneficio

Hoy una conclusión equivocada muere con la sesión. Con vault compartido se propaga
a todas las máquinas al instante **y con autoridad**, porque viene "de la memoria".
Guard y Veracidad valen el doble en un vault multi-máquina que en uno local.

---

## Idea 3 — Agnóstico de plataforma

**Ya lo es.** MCP se volvió el estándar y Claude Code, Codex CLI y GitHub Copilot CLI
lo hablan por HTTP remoto con bearer. La misma URL y el mismo token sirven en los
tres.

**Claude Code** — `~/.claude.json`, clave `mcpServers` en la raíz para alcance
global, o `.mcp.json` en el proyecto (ojo: eso se commitea).

**Codex CLI** — `~/.codex/config.toml`:

```toml
[mcp_servers.cogo]
url = "https://cogo.go.websiteonline.org/mcp"
bearer_token_env_var = "COGO_TOKEN"
```

**GitHub Copilot CLI**:

```bash
copilot mcp add --transport http \
  --header "Authorization: Bearer TU_TOKEN" \
  cogo https://cogo.go.websiteonline.org/mcp
```

o su archivo `~/.copilot/mcp-config.json`, con la misma forma que el de Claude.

> **Detalle de diseño que conviene copiar:** Codex **no acepta el token en el
> archivo de configuración**, exige el nombre de una variable de entorno. Claude
> Code y Copilot lo aceptan en texto plano. Si vas a documentar COGO para terceros,
> mostrá la forma de Codex como la recomendada.

---

## Qué existe en el mercado

| | Multi-máquina | Multi-plataforma | Evidencia y contradicciones |
|---|---|---|---|
| **Memorix** | ✗ SQLite local bajo el proyecto git | ✓ Claude Code, Codex, Copilot, Cursor, Windsurf | parcial: enlaces de origen y chequeo de frescura, **sin** detección de contradicciones |
| **Mem0 / OpenMemory** | ✓ hay nube | ✓ por MCP | ✗ |
| **Zep / Graphiti** | ✓ | es librería para construir agentes, no memoria de agentes de código | grafo temporal: cada hecho registra *cuándo* fue cierto |

**Memorix es el competidor más cercano en forma** —mismo pitch, mismos clientes—
pero es estrictamente local: no tiene sincronización remota ni multi-máquina. Es
justo lo que COGO ya tiene resuelto.

**La intersección de las tres columnas está vacía.**

### Y el hueco está documentado

Un benchmark de 2026 midió que **los sistemas de memoria basados en recuperación
puntúan 0% en detección de contradicciones**, Mem0 incluido con grafo activado. Y
hay papers de este año atacando exactamente el mismo problema: álgebra bitemporal
para resolver contradicciones, actualización de creencias condicionada por
confiabilidad, defensa contra envenenamiento acotada por procedencia.

Que sea materia de papers y no de productos significa dos cosas: no está resuelto,
y el eje elegido es el correcto. **Guard y Veracidad no son adornos de COGO: son lo
único que no tiene nadie.**

Lo único que vale mirar de cerca es **Graphiti**, el grafo temporal de Zep: ya
resolvieron que cada hecho registre cuándo fue cierto, que es medio camino del
"alcance de validez" que falta.

---

## Qué robarle a git, a propósito

La analogía con GitHub es más exacta de lo que suena. Dos cosas:

**Direccionamiento por contenido.** Es lo mismo que el hash como clave de R2: la
referencia prueba que el contenido no cambió.

**Git no resuelve los conflictos, los muestra.** Cuando dos ramas tocan la misma
línea, te para y te obliga a decidir. `supersedes` es esa primitiva. La tentación
va a ser auto-resolver con "gana el último que escribió", y ahí es donde se rompen
todos los demás. **Dos máquinas que afirman cosas opuestas no es un error a
resolver en silencio: es la señal más valiosa que puede dar el sistema.**

---

## Por dónde empezaría

En este orden, por relación entre lo que cuesta y lo que desbloquea:

1. **El cursor en `recall`.** Barato y convierte el vault en canal entre agentes. **✅ HECHO (2026-07-07)**: `recall(since: <cursor>)` devuelve solo las notas cuyo último cambio es posterior al cursor (delta), ordenadas de más nueva a más vieja, y cada respuesta termina con un cursor fresco. Fuente: el historial por nota (`.cogo/history/<id>.jsonl`), ahora con precisión sub-segundo para que dos cambios en el mismo segundo que el cursor no se pierdan. Sin `since` = el bundle completo de siempre + cursor. Avisa además si el mandato cambió desde el cursor. Testeado (unit + e2e por handshake MCP real).
2. **Guard antes de subir cualquier artefacto.** Antes de tocar R2, no después:
   una credencial guardada con hash inmutable no se borra. **✅ HECHO (2026-07-07)**:
   `internal/secretscan` corre antes de cada `Put` y, por defecto, **rechaza**
   guardar si detecta un secreto (claves AWS/R2/Google/GitHub/Slack, `sk-`/`cfut_`,
   private keys, JWT, credenciales en URL, asignaciones secreto=valor); opt-in a
   `redact:true` para guardar una copia censurada.
3. **R2 con clave por SHA-256**, y `verify` recalculando el hash. Es lo que
   convierte la veracidad en algo computado. **✅ HECHO (2026-07-07)**: store
   content-addressed (`internal/artifact`, backend R2 + disco, SigV4 propio) +
   tool MCP `stash` / `POST /api/artifact` que devuelven `artifact://<sha>` para
   citar como evidencia; `ResolveEvidence` chequea la existencia en el store
   (presente → resuelto, ausente → roto), así el color se **recomputa**. Como la
   clave ES el hash, nunca driftea. Verificado end-to-end contra R2 real.
   **Tramo 3 HECHO (2026-07-07)**: (a) la **Papelera llega al store con conteo de
   referencias** — al purgar una nota, sus artefactos se borran del store solo si
   ninguna otra nota (viva o en papelera) los sigue citando (`core.ReferencedArtifacts`);
   como el store deduplica, un blob compartido sobrevive hasta que se purga su
   último citador (verificado: dos notas comparten un artefacto → purgo una,
   sobrevive; purgo la otra, se borra). (b) **UI de subida** en el visor: botón
   "adjuntar archivo" en el editor (pasa por el guard, ofrece redactar si hay
   secreto) + link "descargar" en la vista de nota para las evidencias
   `artifact://`. **Punto #3 completo.**
4. **Alcance de validez en la nota** —sistema operativo, commit, runtime—. Sin
   esto, la memoria compartida entre máquinas se contamina sola.
5. **Tokens por agente.**
6. **Leases**, cuando haya dos agentes tocando el mismo repositorio. **✅ HECHO
   (2026-07-07)**: `internal/lease` (permisos con nombre y vencimiento en
   `.cogo/leases.json`) + tool MCP `lease` (acquire/release/list; el holder por
   defecto es tu identidad autenticada) + `GET /api/leases`. `acquire` falla si
   otro lo tiene (te dice quién y hasta cuándo); expiran solos (un holder que
   crashea no traba el vault); re-entrante para el mismo holder (renovar). Como
   git: hace visible la colisión, no la bloquea físicamente. Verificado e2e MCP.

---

## Fuentes

- Memorix — https://github.com/AVIDS2/memorix
- Mem0 / OpenMemory — https://mem0.ai/openmemory
- Mem0 vs Zep — https://mem0.ai/blog/mem0-vs-zep
- MnemeBrain Benchmark, detección de contradicciones — https://mnemebrain.github.io/mnemebrain-benchmark/
- TOKI, álgebra bitemporal — https://arxiv.org/html/2606.06240
- Actualización de creencias y procedencia — https://arxiv.org/abs/2606.22030
- MCP en Copilot CLI — https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers
- MCP remoto en Codex — https://github.com/github/github-mcp-server/blob/main/docs/installation-guides/install-codex.md
