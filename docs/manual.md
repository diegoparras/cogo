# COGO — Manual para dummies

Un manual sin vueltas. Si en algún momento pensás "esto es más difícil de lo que debería",
avisá: COGO está hecho para que sea una pavada usarlo.

---

## 1. ¿Qué es COGO, en una frase?

**Un semáforo.** COGO le pone un color —🟢 verde, 🟡 amarillo, 🔴 rojo— a dos cosas que
solo, mirándolas, no podés juzgar bien:

1. **Lo que sabés de tu proyecto:** ¿este dato es confiable o es una corazonada?
2. **Lo que te dice una IA:** ¿esta respuesta es sincera o te está *empujando* a algo?

No tenés que ser experto en nada. Mirás el color y decidís. Eso es todo.

---

## 2. Las dos mitades de COGO

COGO hace dos cosas distintas. Pensalas como dos herramientas en la misma caja.

### 🗒️ Mitad A — La memoria con semáforo
Anotás lo que vas sabiendo de tu proyecto ("la base de datos está en tal lado", "el error
lo causa esto", "decidimos aquello"). COGO le pone un color de confianza a cada nota, y lo
**calcula solo** (no lo elegís vos). Así, cuando volvés dentro de tres semanas —o cuando una
IA lee tus notas para ayudarte— se sabe de un vistazo en qué se puede confiar y en qué no.

### 🛡️ Mitad B — Guard, el detector de manipulación
Pegás lo que te respondió una IA (ChatGPT, Claude, el que sea) y COGO te dice si te está
manipulando: si te mete miedo, si te apura, si te hace sentir culpa, si te niega algo que
dijo antes. No censura a la IA: **te avisa a vos**, y vos decidís.

---

## 3. Cómo lo prendo (la parte de "abrir el programa")

COGO se usa desde el navegador, como una página web, pero corre en tu propia máquina.
Elegí **una** de estas dos formas:

**Opción fácil (con Docker):** copiá y pegá esto en una terminal:
```
docker run -d -p 127.0.0.1:8095:8080 -v cogo-vault:/vault -e COGO_ALLOW_INSECURE=1 ghcr.io/diegoparras/cogo
```
Después abrí el navegador en **http://localhost:8095**. Listo. (Para instalarlo en
un **servidor** —EasyPanel, VPS— seguí [`instalacion.md`](instalacion.md): ahí va con
token, que es lo seguro.)

**Opción sin Docker (un solo archivo):** si tenés el programa `cogo`, escribí:
```
cogo serve -http :8080 -vault ./vault
```
Y abrí **http://localhost:8080**.

> ¿"Terminal"? Es esa ventana negra de escribir comandos. Si nunca la usaste, pedile a
> alguien que te la abra la primera vez; después es siempre el mismo comando.

Cuando abras la dirección en el navegador, vas a ver arriba unas **pestañas**. Esa es toda
la aplicación.

---

## 4. Las pestañas, una por una

- **Vault** ("bóveda"): tus notas, cada una con su color. Es la pantalla principal.
- **Frescura**: te avisa qué notas están "venciendo" (las cosas cambian; una nota vieja deja
  de ser confiable). Tiene un botón "revalidar" para renovarlas.
- **Pack**: junta tus notas de un tema en un texto ordenado para copiar y pegárselo a una IA.
- **Grafo**: un dibujo de cómo se relacionan tus notas. Lindo de ver, opcional.
- **Revisión**: busca problemas (enlaces rotos, contradicciones entre notas).
- **Guard**: el detector de manipulación (la Mitad B). Le dedicamos la sección 6.

---

## 5. Usar la memoria (Mitad A), paso a paso

**Crear una nota:**
1. En la pestaña **Vault**, tocá **"+ Nueva nota"**.
2. Escribí lo que sabés (empezá con `## Claim` y abajo tu frase; es solo el título de la
   sección, no te compliques).
3. Si tenés una **prueba** de que es cierto (un log, un archivo, una captura), sumala en
   "Evidencia". Si no tenés prueba, no pasa nada: la nota va a nacer 🔴 roja, que quiere
   decir "esto es una suposición, no te fíes todavía".
4. Guardá.

**Entender el color** (COGO lo calcula, vos no lo elegís):

| Color | Qué significa |
|---|---|
| 🟢 verde | Verificado. Podés confiar. |
| 🟡 amarillo | Probable. Hay algo, pero falta confirmarlo. |
| 🔴 rojo | Suposición o corazonada. No te apoyes en esto. |

**Subir el color de una nota:** sumá evidencia y, cuando la chequeaste de verdad, apretá
**"verificar"**. La nota se pone más verde. Si la dejás vieja, sola se va poniendo amarilla
y después roja. Es honesto: refleja el estado real.

---

## 6. Usar Guard (Mitad B), paso a paso

Guard responde una pregunta: **"esta respuesta de la IA, ¿me está empujando?"**

**Paso 1 — Declará tu mandato (una vez).**
Tu "mandato" es lo que querés y —sobre todo— **lo que NO estás dispuesto a hacer**. Ejemplo
de línea roja: *"no renuncio a mi trabajo sin otra oferta firmada"*. Esto es importante:
sin saber qué NO querés, es imposible distinguir un buen consejo de un empujón. Se escribe
una vez y queda guardado.

**Paso 2 — Pegá la conversación.**
- En **"Conversación previa"** ponés lo que se habló hasta ahí, un mensaje por renglón:
  `U:` lo que dijiste vos, `M:` lo que respondió la IA.
- En **"Turno a analizar"** ponés el último mensaje de la IA, el que querés revisar.

**Paso 3 — Apretá "Radiografiar"** y leé el resultado.

Vas a ver un color y unas tarjetas. Cada tarjeta te dice **qué táctica encontró**, te
muestra la **frase exacta**, te hace unas **preguntas para pensar** y te da la
**contramedida**. Los colores:

- 🟢 **verde** — sin señales, tranquilo.
- 🟡 **amarillo** — hay persuasión o toca tu línea roja. Mirá de cerca, pero decidí vos.
- 🔴 **rojo** — hay **recibos**: la IA se contradice con algo que dijo antes, y COGO te
  muestra las dos frases lado a lado. Eso ya no es tu palabra contra la suya.

**Ejemplo real para probarlo.** Pegá esto:
- *Conversación previa:*
  ```
  U: ¿Me conviene renunciar a mi trabajo?
  M: Sí, deberías renunciar cuanto antes, te está frenando la carrera.
  ```
- *Turno a analizar:*
  ```
  Yo nunca dije que tengas que renunciar; estás confundiendo lo que dije. Decidí ya, antes de que sea tarde.
  ```
COGO lo va a marcar 🔴 y te va a mostrar el **recibo**: la IA sí te dijo "deberías renunciar
cuanto antes", y ahora lo niega. Ese es el momento en que un detector te salva de dudar de
tu propia memoria.

---

## 7. Preguntas que todos hacen

**¿COGO decide por mí?** No. Nunca. Te muestra y vos decidís. Está hecho a propósito para
no reemplazar tu criterio, sino para darte con qué ejercerlo.

**¿Necesito conectar una IA para que funcione?** No para lo básico. La memoria y buena parte
de Guard andan sin ninguna IA. Si conectás una (en *Ajustes · Modelo IA*), Guard se vuelve
más agudo: caza manipulaciones más sutiles. Es opcional.

**¿Guard atrapa todo?** No, y no te mentimos con eso. Atrapa lo grueso siempre, y —con una IA
buena conectada— la mayoría de lo sutil. Tratalo como un segundo par de ojos muy bueno, no
como un guardaespaldas infalible.

**¿Se sube mi información a algún lado?** No, salvo que vos conectes una IA externa. Por
defecto todo queda en tu máquina.

**¿Y si me marca algo que en realidad estaba bien?** Puede pasar (amarillo de más). Por eso
el amarillo dice "mirá de cerca", no "es manipulación". El rojo, en cambio, es casi siempre
un recibo real, con las pruebas a la vista.

---

## 8. Diccionario mínimo

- **Nota:** una cosa que sabés de tu proyecto, guardada con su color.
- **Color de confianza:** 🟢/🟡/🔴, calculado por COGO, no elegido por vos.
- **Verificar:** decirle a COGO "esto ya lo chequeé de verdad" → sube el color.
- **Mandato:** tu objetivo + tus líneas rojas (lo que NO estás dispuesto a hacer).
- **Línea roja:** un límite tuyo que declaraste. Guard avisa cuando un mensaje lo toca.
- **Radiografía:** el resultado de Guard sobre un mensaje de la IA.
- **Recibo:** la prueba de que la IA se contradice, con las dos frases lado a lado. Es lo
  único que Guard marca 🔴, porque es lo único que puede **demostrar**.

---

## 9. Para los que quieren rosquear (opcional, no-dummies)

Si te copa la parte técnica: COGO también es un **servidor MCP** (para que tu agente de IA
—Claude Code, Cursor— lo use solo, con la herramienta `guard`) y tiene un **CLI** completo
(`cogo add`, `pack`, `search`, `verify`, `lint`…). Todo eso está en el
[README](../README.md) y en los documentos de diseño (`docs/`). Pero para usar COGO **no
hace falta nada de esto**: con el navegador alcanza.

---

*COGO es parte de la Suite Escriba. Hecho para que la primera vez que lo abras digas
"ahh, era una pavada".*
