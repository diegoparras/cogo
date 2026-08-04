# COGO — guía para agentes de IA

> Si sos un agente de código (Claude Code, Codex, Copilot, OpenCode, Antigravity…)
> y este proyecto tiene COGO conectado, esto es lo que necesitás saber. Está
> escrito para que lo leas vos, no tu humano: la versión para humanos es
> `COGO-manual.pdf`.

---

## 1. Qué es COGO y por qué te conviene

COGO es la **memoria compartida** del proyecto. No guarda archivos: guarda
**afirmaciones verificables**, cada una con un **color de confianza que COGO
computa** a partir de su evidencia.

La regla más importante, y la que cambia cómo trabajás:

> **Vos no decidís el color. Lo obedecés.**

Esto no es una restricción arbitraria: es lo que te ahorra trabajo. En vez de
releer medio repositorio para reconstruir qué se decidió y qué se probó, pedís
el contexto ya juzgado y actuás. Lo que otro agente verificó ayer, vos lo
consumís hoy sin re-derivarlo.

### Los tres colores

| Color | Qué significa | Qué hacés |
|---|---|---|
| 🟢 **verde** | verificado: evidencia observada, check pasado, fresco | Te apoyás con confianza. |
| 🟡 **amarillo** | probable: evidencia reportada, check sin correr, vencida, o cambió lo que cita | Lo usás **diciendo que es probable**. |
| 🔴 **rojo** | no confiable: sin evidencia, cita rota o contradicción abierta | **No te apoyás.** Ya viene en cuarentena. |

`pack` no se limita a rotular el rojo: lo **saca del cuerpo del contexto** y lo
aísla en una sección "do not rely". Si algo llega ahí, tratalo como
"esto puede estar mal", incluso si suena razonable.

**Sobre "cambió lo que cita":** cuando una nota apunta a `archivo.go:42`, COGO
guarda el TEXTO de esa región, no solo la fecha del archivo. Si el archivo cambia
en otro lado, o si lo citado sigue igual pero se corrió de línea, la nota **no
baja de color** — cambió el archivo, no la afirmación. El amarillo aparece
únicamente cuando cambió lo que la nota citaba. Vale la pena decirlo porque
implica lo contrario: si ves ese amarillo, el cambio es real y vale la pena
mirarlo.

---

## 2. El protocolo (obligatorio)

1. **Consultá antes de actuar.** Antes de responder o cambiar algo, pedí
   contexto con `pack`. Es lo primero, siempre.
2. **Respetá el color.** Ver la tabla de arriba.
3. **Capturá lo que verifiques.** Cuando confirmes algo nuevo, guardalo con
   `capture`: claim + evidencia real + el check mínimo que lo probaría.
   **No escribas el color**: COGO lo computa.
4. **No pises el verde.** Si ya existe una nota verde, no la sobrescribas a
   ciegas: verificala de nuevo o usá un id nuevo.
5. **El rojo no se "arregla" escribiendo.** Una contradicción o una cita rota
   se resuelve corrigiendo la nota o la evidencia, nunca reformulando el texto
   para que suene mejor.

### Qué va en COGO y qué no

| | Dónde vive | Qué |
|---|---|---|
| **Archivos** | el repositorio (git) | código, documentación, configuración |
| **Juicio** | **COGO** | decisiones, restricciones, bugs conocidos, runbooks |

**No metas archivos del proyecto en COGO.** El repo ya los versiona con historia
y `diff`. En COGO va lo que *no* está en los archivos o costaría re-derivar: por
qué se decidió algo, qué restricción rige, qué ya se probó y falló.

---

## 3. Anatomía de una nota

```yaml
id: tienda-checkout-400ms      # estable; si lo omitís se deriva del claim
type: constraint               # decision|bug|runbook|architecture|constraint|command|mistake
project: tienda                # partición: filtrá SIEMPRE por proyecto
evidence:
  - kind: test_result          # ver la tabla de tipos, abajo
    ref: "k6 run checkout.js -> p95=312ms (2026-07-26)"
check:
  test: "k6 p95 del checkout < 400ms"
  status: passed
author: token:codex            # lo pone COGO desde tu identidad
scope:                         # opcional: dónde vale esta afirmación
  os: linux
```

```markdown
## Claim
El checkout **no puede superar los 400 ms en p95**. Es la restricción que
ordena las decisiones de caché y de índices.
```

### Qué hace una buena nota

- **Claim declarativo**: una afirmación que se pueda contradecir. Si no se puede
  contradecir, no es una nota.
- **Evidencia real**: un archivo con su línea, un comando con su salida, un log
  con su hora. No "según recuerdo".
- **Check mínimo**: el test más chico que la probaría. Si no se te ocurre
  ninguno, probablemente sea una opinión y no una nota.

### Tipos de evidencia, de más fuerte a más débil

| Tipo | Ejemplo |
|---|---|
| `direct_log`, `command_output`, `test_result`, `file_read` | **observada**: puede llevar la nota a verde |
| `doc`, `testimony` | **reportada**: el techo es amarillo |
| `inference`, `hypothesis`, `absence` | **razonada**: el techo es amarillo |

---

## 4. Cómo citar evidencia

Esto define si tu nota puede llegar a verde. Tres formas, de menos a más fuerte:

```
src/checkout.go:42                                  ruta local
github://owner/repo@main/src/checkout.go:42         del repositorio  ← preferida
artifact://<sha256>                                 artefacto guardado por COGO
```

- **Ruta local**: se resuelve contra la raíz del proyecto. Anda en una máquina
  con el repo clonado; en una instancia hosteada **no hay working copy**, así
  que la cita queda sin verificar. Evitala si podés.
- **`github://`**: COGO baja el archivo por la API, confirma que la cita existe
  y guarda el hash del blob. Si citás una **rama** y ese archivo cambia, la nota
  **cae sola a amarillo** pidiendo re-verificación. Si citás un **commit fijo**,
  la cita es inmutable.
- **`artifact://`**: para lo que hoy se pierde (la salida completa del comando
  que falló, un CSV que prueba un conteo). Guardalo con `stash` y citá la
  referencia que te devuelve; la clave *es* el hash del contenido, así que la
  referencia prueba que nadie lo editó.

> ⚠️ **Nunca** guardes credenciales en una nota ni en un artefacto. COGO escanea
> el contenido antes de guardarlo y **se niega** si detecta algo que parece una
> clave: un artefacto guardado por su hash no se puede borrar de la historia.

---

## 5. Las herramientas

### Lo que usás todo el tiempo

**`pack(query, project, token_budget, env)`** — contexto coloreado sobre un
tema. Lo primero que llamás. `env` describe tu entorno (`{"os":"linux"}`) para
que COGO te marque las notas cuyo alcance no coincide con el tuyo.

**`capture(type, body, evidence[], check_test, project, scope, …)`** — guardar
un hallazgo. Nunca pases el color.

**`verify(id)`** — marcar el check como pasado. COGO re-estampa el hash de la
evidencia y recalcula. Hacelo **solo si realmente corriste el check**.

**`search(query, project)`** — listar notas por tema. Si hay embeddings
configurados busca por significado; si no, por palabra.

**`open(id)`** — la nota entera con su color recalculado y, si la hay, la traza
de con qué otra nota choca.

### Al arrancar y al cerrar

**`recall(project, since)`** — re-anclarte. Sin argumentos devuelve el
**mandato** del usuario (sus líneas rojas) y las decisiones verdes vigentes,
más un **cursor**. Llamalo:

- al **empezar** la sesión;
- después de una **compactación de contexto** (es cuando se pierden las
  restricciones sin que te des cuenta);
- con `since: "<cursor previo>"` para recibir **solo lo que cambió** desde
  entonces, en vez de releer toda la memoria.

**`reflect(summary)`** — al terminar una tarea, pasale un resumen de lo que
hiciste y verificaste; te propone qué vale la pena capturar. Vos decidís.

### Coordinación con otros agentes

**`lease(action, name, ttl_seconds, note)`** — antes de una tarea **no
idempotente** (una migración, un deploy, una edición masiva):

```
lease(action:"acquire", name:"migrar-db", ttl_seconds:900, note:"migración 0042")
```

Si otro agente lo tiene, COGO te dice **quién** y **hasta cuándo**: **no
arranques**. Al terminar, `lease(action:"release", name:"migrar-db")`. Expiran
solos, así que un agente que se cuelga no traba el vault.

### Evidencia y radiografías

**`stash(content | content_base64, content_type, redact)`** — guardar un
artefacto por su hash y recibir la referencia para citarlo.

**`guard(turn, transcript, goal, red_lines, steelman)`** — radiografía
anti-manipulación de un turno de un modelo.

**`xray(text)`** — radiografía de veracidad: separa las afirmaciones y marca
compromiso, evidencia y falsabilidad.

**`archive(id)` · `restore(id)` · `remove(id)`** — ciclo de vida de una nota.
Preferí `archive` (entierra sin borrar) sobre `remove`.

---

## 6. Trabajás con otros agentes

El vault es compartido. Otros agentes escriben en él mientras vos trabajás.

- **Sincronizate por cursor.** Guardá el cursor que te da `recall` y pasalo como
  `since` la próxima vez: recibís el delta, no toda la memoria.
- **Cada nota tiene autor.** COGO registra qué agente escribió cada claim desde
  tu identidad autenticada. No lo falsifiques.
- **Declarás alcance cuando importa.** Si tu hallazgo depende del sistema
  operativo, del commit o de una versión, ponelo en `scope`. Una verdad en
  Windows puede ser falsa en Linux, y sin `scope` esa nota llega verde a la otra
  máquina.
- **Filtrá por proyecto.** Usá siempre `project` en `pack`, `search`, `capture`
  y `recall`: sin eso, mezclás memoria de proyectos que no tienen nada que ver.

---

## 7. Errores comunes (no los cometas)

| ❌ Esto | ✅ Esto |
|---|---|
| Actuar sin llamar a `pack` primero | Consultar la memoria antes de decidir |
| Escribir `confidence: green` en una nota | Dejar que COGO lo compute |
| `verify` sin haber corrido el check | Verificar solo lo que probaste |
| Capturar "el usuario dijo que…" como si fuera verificado | `kind: testimony`, que topea en amarillo |
| Volcar un archivo del repo como nota | Citarlo con `github://` |
| Reescribir una nota roja para que suene mejor | Corregir la evidencia o resolver la contradicción |
| Pisar una nota verde con un id repetido | Re-verificarla, o usar un id nuevo |
| Correr una migración sin `lease` | Tomar el permiso primero |
| Guardar un log con credenciales | Limpiarlo antes, o usar `redact` |

---

## 8. Un ciclo completo

```
1. recall(project:"tienda")
   → mandato del usuario + decisiones verdes + cursor

2. pack(query:"performance del checkout", project:"tienda", env:{"os":"linux"})
   → lo verificado, lo probable, y el rojo en cuarentena

3. [trabajás: leés el repo, probás, medís]

4. lease(action:"acquire", name:"migrar-db")   ← solo si vas a hacer algo no idempotente
   ...
   lease(action:"release", name:"migrar-db")

5. stash(content:"<salida del comando que falló>")
   → artifact://a1b2c3…

6. capture(
     type:"bug", project:"tienda",
     body:"## Claim\nEl worker de emails pierde mensajes con la cola sobre 5k.",
     evidence:[{kind:"direct_log", ref:"artifact://a1b2c3…"}],
     check_test:"encolar 5k mails y contar los entregados",
     scope:{os:"linux"}
   )

7. verify("...")   ← solo si corriste el check y pasó

8. reflect(summary:"...")  → ¿algo más que valga la pena guardar?
```

---

## 9. Lo que COGO nunca hace

Para que no lo esperes:

- **No decide por vos.** Te da el color y el motivo; la decisión es tuya (y de
  tu humano).
- **No censura al modelo.** El Guard nombra tácticas y da preguntas críticas;
  no bloquea ni reescribe.
- **No resuelve contradicciones en silencio.** Dos notas opuestas quedan las dos
  en rojo hasta que un humano decide. Es la señal más valiosa del sistema, no un
  error a limpiar.
- **No inventa evidencia.** Si no puede verificar algo, lo deja "sin verificar"
  en vez de castigarlo: nunca marca como roto lo que simplemente no pudo ver.

---

COGO es software libre: `github.com/diegoparras/cogo`
Ecosistema Escriba · Diego Parras
