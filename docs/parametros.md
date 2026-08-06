# Los parámetros del motor · el modo deidad

> Cómo COGO decide cuánto dura fresca una nota, cuánto respaldo pide cada tipo
> de acción, y cuándo deja de creerle a quien declara.

---

## La idea

Todo motor de reglas tiene constantes. Cuántos días dura fresca una decisión,
cuánta evidencia hace falta para autorizar un borrado, cuántas observaciones se
necesitan antes de creerle a una estadística.

Repartidas por el código son invisibles: nadie sabe que están, nadie sabe qué
pasa si se mueven, y quien las quiere cambiar tiene que recompilar.

En COGO están todas en un registro: **veinte parámetros**, cada uno con su
etiqueta, qué hace, en qué unidad, entre qué valores es válido y qué se afloja
si se mueve. El panel del visor **se genera de ahí** — no hay una lista de
controles escrita a mano que pueda desincronizarse de lo que el motor lee.

### Los dos modos

El default es que esto no exista para vos. COGO decide, y decide bien: ningún
flujo normal pide tocar un número. **Un vault que nadie configuró no tiene
siquiera archivo de parámetros.**

Y cuando hace falta, está todo. No un subconjunto seguro, no "opciones
avanzadas" a medias: los veinte, con su efecto escrito y con los que aflojan el
sistema marcados como tales.

> Esconder controles porque el usuario podría lastimarse es condescendiente. No
> necesitarlos porque los defaults son buenos es diseño.

**Dónde:** Ajustes → *ver los controles*. Cada cambio queda en la auditoría con
quién lo hizo, y se guarda en `.cogo/parametros.json` — que solo contiene **lo
que difiere del default**, así actualizar COGO mueve los defaults hacia adelante
sin pisar lo que alguien decidió a mano.

---

## Frescura · cuánto dura fresca cada cosa

Una nota no envejece igual según lo que afirme. Pasada su ventana baja a
amarillo; al doble, expira a rojo.

| Tipo | Días | Por qué |
|---|---:|---|
| `constraint` | 365 | invariantes: lo que no puede dejar de ser cierto |
| `decision` / `architecture` | 180 | cambian, pero no seguido |
| `runbook` | 90 | los procedimientos se desactualizan |
| `bug` | 60 | |
| `command` | 30 | lo que más rápido queda viejo |
| cualquier otro | 90 | default conservador |

---

## Olvido · cuándo una nota sale del camino

COGO calificaba todo lo que entraba y no sacaba nada nunca. El color aísla lo
rojo pero no lo elimina: sigue existiendo, sigue costando, sigue apareciendo. Un
vault a tres años son miles de notas, casi todas vencidas, todas ahí — y lo
muerto tapa a lo vivo.

**Olvidar por antigüedad sería el error obvio**, porque la nota más vieja del
vault puede ser la que más se consulta. COGO puede hacerlo mejor porque sabe
cosas que un contador de fechas no. Una nota se vuelve **latente** cuando se dan
todas a la vez:

| | Por qué |
|---|---|
| expiró | pasó el **doble** de su ventana. Una apenas vencida todavía se re-verifica |
| nadie depende de ella | si algo se apoya, sacarla lo dejaría apoyado en el aire |
| nadie la consultó en 180 días | la condición que hace que esto no sea olvidar por edad |

Y **nunca** salen del camino: las restricciones (sostienen todo lo demás), las
fijadas a mano (`pinned: true`), las que tienen una contradicción abierta
(esconderla escondería el conflicto) y las preguntas abiertas.

**Latente no es borrada.** La nota sigue en el vault, sigue siendo un archivo,
sigue abriéndose por su id, y se ve en el visor con su motivo. Lo que cambia es
que deja de entrar en el `pack` y en las búsquedas.

**Y se calcula, no se escribe** — igual que el color. Nadie marca una nota como
latente: la condición se evalúa cada vez. Por eso despertar no tiene ceremonia:
consultala y deja de estar sin consultar, así que deja de ser latente. No hay un
estado que alguien tenga que acordarse de revertir.

> Para que esto no sea adivinar, COGO registra **qué notas se consultan**: las
> que entran en un `pack` y las que se abren por su id. Aparecer en una búsqueda
> no cuenta — eso mediría coincidencias léxicas, no uso. El registro guarda su
> propia fecha de inicio, así que instalar esta versión no vuelve latente a medio
> vault el primer día: una nota sin registro no es una que nadie consultó, es una
> que nadie consultó *desde que se empezó a mirar*.

---

## Origen · quién decidió

Un agente propone Fastify. Vos decís "dale". El agente captura *"se decidió usar
Fastify"*, con su autor y su evidencia. Mañana lo lee de vuelta como un hecho
establecido del proyecto y construye encima. En cada vuelta, una opinión se lava
en hecho.

Los ejes que COGO ya tenía no lo ven: la evidencia puede ser impecable —un
`file_read` del `package.json` que el propio agente escribió— y la procedencia
dice quién corrió el check, no quién tuvo la idea.

Por eso las notas normativas (`decision`, `constraint`) llevan **origen**:

```
origin: human       lo decidió una persona
origin: agent       lo propuso el agente
origin: instrument  salió de un instrumento: nadie lo eligió, se midió
```

Solo esos dos tipos. Un `bug` o un `runbook` describen cómo es el mundo, y ahí
la evidencia responde; una decisión afirma que alguien **eligió**, y ninguna
salida de comando puede probar una elección.

**No baja el color, y es deliberado.** Un techo obligaría a ratificar a mano cada
decisión que tome un agente, y COGO se juega en no agregar tareas: una
herramienta que pide trabajo para seguir siendo confiable termina no usándose. La
etiqueta da casi todo el valor — el que lee sabe que eso se puede revisar. Y si
con el tiempo resulta que verla siempre lleva a actuar, ponerle techo es un
cambio de una línea; sacárselo después de acostumbrar a un equipo, no.

En el pack se ve así:

```
- origin: **proposed by an agent** — no human chose this; it is open to revision
```

---

## Acciones · cuánto respaldo pide cada una

Acá vive la fase 7, y es la mitad que le faltaba al sistema. COGO decía cuánto
vale cada cosa que sabe; **cuánto tiene que valer depende de para qué**.

| Clase | Qué es | Pide por default |
|---|---|---|
| informativa | responder, explicar, resumir | `asserted` |
| reversible | editar, crear, commitear | `check_declared` |
| costosa | deploy, migración, gasto | `claimed_passed` |
| irreversible | borrar, publicar, enviar, force push | **`verified`** |

La línea que importa es la última: **lo irreversible es la única clase que exige
un check ejecutado y no declarado**. Ahí la palabra de un agente no alcanza, y
todo el aparato de las fases anteriores —el runner, la procedencia, el
retículo— existe para poder trazar esa línea y que signifique algo.

### Por qué no alcanza con que el agente declare la clase

Porque el agente que quiere hacer algo es exactamente quien tiene el incentivo
de clasificarlo bajo. *"Voy a limpiar unos temporales"* puede ser un `rm -rf`.

Así que la clase se decide **dos veces** —lo que el agente declara y lo que se
infiere del texto— y **gana la más estricta**. Un agente puede subir la
exigencia sobre sí mismo; no puede bajarla.

```
authorize("limpiar unos temporales con rm -rf en la carpeta de build",
          class: "informative", notes: [...])

NOT AUTHORIZED — una acción irreversible necesita respaldo verified
action class: irreversible (declarada "informativa", pero el texto dice
              irreversible (borrado de archivos): manda la más estricta)
```

---

## Coordinación · cuándo un agente se entera de otro

| parámetro | default | qué hace |
|---|---|---|
| `coordinacion.ventana_minutos` | 30 | hasta cuándo cuenta como "ahora mismo". **En 0 no se avisa nunca.** |
| `coordinacion.bloquear_por_permiso` | encendido | si una acción que nombra un permiso ajeno se **rechaza** o solo se avisa |

COGO no puede interrumpir a un agente: MCP es pregunta y respuesta. Lo que hace
es **contestar más de lo que le preguntaron** — el aviso de que hay otro
trabajando acá viaja colgado del `pack` y del `authorize`, que son justo los
llamados que preceden a una acción.

**La ventana es el compromiso entero.** Muy chica deja pasar colisiones reales;
muy grande avisa de gente que ya terminó, y *un aviso que aparece siempre es un
aviso que nadie lee*. Por eso el bloque no sale nunca que no haya algo concreto
que decir: si el otro está en otro proyecto, o el único permiso vigente es el
propio, la respuesta sale como si el parámetro estuviera apagado.

**El bloqueo es el único rechazo de COGO que no habla de evidencia.** `authorize`
siempre preguntó *"¿te alcanza lo que sabés?"*; con esto pregunta también *"¿ya
lo está haciendo otro?"*. Verificar mejor no lo destraba — se destraba esperando
o hablando.

El criterio es una **coincidencia de nombre**, no una inferencia: si alguien tomó
el permiso `migrar-db` y la acción dice "migrar-db", eso no es una sospecha, es
la misma cosa. Un criterio más flojo produciría bloqueos falsos, y **un bloqueo
falso en una herramienta de seguridad se paga con que la apaguen**.

Apagar `bloquear_por_permiso` deja que dos agentes hagan la misma migración al
mismo tiempo, cada uno creyendo que es el único.

---

## Sello · probar que es el mismo registro

| parámetro | default | qué hace |
|---|---|---|
| `sello.activo` | apagado | habilita publicar la cabeza de la cadena afuera |
| `sello.url` | vacío | a dónde se publica (vacío = `manual`: imprime y no manda nada) |
| `sello.cada_eventos` | 500 | cada cuántos eventos conviene sellar de nuevo |

La cadena de hashes detecta que alguien **alteró** un evento viejo. No detecta
que el dueño del vault haya **rehecho la historia entera**, porque ahí recalcula
todos los digests y la cadena queda internamente perfecta.

Sellar publica la cabeza en un lugar que el dueño no controla, y es lo único que
cierra ese hueco: un sello que ya no coincide es la prueba de que el registro se
reescribió después de publicarse.

Viene apagado porque publicar es una decisión, no un default: cada sello revela
que este vault existe y cuánto se escribió en él.

---

## Los dos módulos que vienen apagados

Calibración y supervivencia necesitan datos que solo se juntan con el uso.
Están **implementados, desplegados y midiendo**, pero no tocan ningún color
hasta que alguien los encienda a sabiendas. En el panel, la sección *Lo que el
vault aprendió* muestra qué dirían si estuvieran encendidos y con cuánto
respaldo — que es la información con la que se decide encenderlos.

### Calibración por emisor

Cuando alguien declara "el check pasa" y después el check ejecutado falla, eso
queda registrado. Con suficientes casos se puede dejar de creerle igual a todos.

**El denominador es lo importante.** La tentación es dividir las desmentidas por
el total de declaraciones, y estaría mal: la mayoría de las declaraciones nunca
se ejecutan, y contarlas como aciertos le regalaría una tasa de error diminuta a
cualquiera que declare mucho y verifique poco — el comportamiento que habría que
castigar. El denominador son las declaraciones **que fueron puestas a prueba**.

**Y hay un intervalo, no un promedio.** Dos desmentidas sobre tres pruebas da
67%, que suena catastrófico y no significa nada. Se penaliza por la cota
inferior de Wilson: el piso de la tasa de error que los datos *sostienen*. Con
tres pruebas ese piso es bajísimo aunque las tres hayan fallado. Una muestra
chica no acusa a nadie.

Un emisor penalizado deja de convertir su palabra en `claimed_passed`. **No
puede bajar un `verified`**: un check ejecutado no depende de la reputación de
nadie — lo vio una máquina, y el código de salida es el código de salida.

### Ventanas por supervivencia

Los 180 días de `decision` no salen de ningún lado. Son una intuición razonable
y nada más. El vault tiene la respuesta: cada nota desmentida o contradicha dice
cuánto duró siendo cierta.

**No es un promedio.** La mayoría de las notas no falló *todavía*; promediar
solo las que fallaron es preguntar cuánto vive la gente encuestando velorios.
Las notas vivas son observaciones **censuradas**: no se sabe cuánto van a durar,
pero sí que duraron al menos lo que llevan. Kaplan-Meier usa esa información
parcial en vez de tirarla.

La ventana va donde la curva cruza el cuantil elegido. Con el default del 20%:
fresca mientras 4 de cada 5 notas parecidas seguían siendo ciertas. Es una
elección de tolerancia al riesgo, no un hecho — y por eso es un parámetro.

Cuando no hay suficientes notas de un tipo, o cuando la curva nunca baja hasta
el corte, **no contesta** y ese tipo sigue con la tabla. Una estimación sobre
cuatro casos sería peor que la intuición que reemplaza.

---

## Las invariantes

Cinco propiedades del motor, verificadas sobre cientos de vaults generados al
azar —con ciclos, dependencias arbitrarias y evidencia mezclada— en cada
ejecución de la suite:

1. **Determinismo.** El mismo vault da el mismo resultado siempre. No es
   trivial: el motor recorre mapas de Go, cuyo orden de iteración es
   deliberadamente aleatorio.
2. **La propagación solo baja.** Apoyarse en algo no puede volverte más
   confiable de lo que sos.
3. **Una contradicción nunca mejora.** Registrar un problema no puede subirle el
   estado a nadie, o el sistema estaría premiando que se oculten.
4. **Quitar evidencia nunca sube.** Lo que se afirma se afirma porque hay con
   qué.
5. **Nadie llega a `verified` sin haber ejecutado.** Es la línea de la que
   dependen los umbrales de las acciones irreversibles.

> El spec pedía modelar todo esto en TLA+. Un modelo formal es un artefacto
> aparte, y lo que verifica es el modelo: nada garantiza que el modelo y el Go
> digan lo mismo, y en cuanto divergen —lo hacen siempre— el model checker pasa
> a certificar un sistema que no es el que corre. Estas propiedades son más
> débiles (cubren los casos que se generan, no todos) y son verdaderas **del
> código que se despliega**. Para un motor que cabe en un proceso, es el
> intercambio correcto.

La invariante 3 encontró un defecto real el día que se escribió: abrir una
contradicción sobre una nota ya refutada la movía de `refuted` a `contradicted`
—hacia arriba— o sea que registrar un problema mejoraba la nota. Los dos estados
son rojos, así que ningún test de color lo habría visto nunca.
