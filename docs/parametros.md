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
