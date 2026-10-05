# COGO — Fundamento teórico (por qué la regla de hierro)

> Este documento resume el marco conceptual detrás de COGO: el *porqué* de la regla que
> gobierna sus dos motores. La formulación completa vive en un **working paper en
> preparación** del autor (Diego Parrás, *Ingeniería del desconocimiento*); todavía **no
> está publicado**, así que acá va lo justo para entender el diseño, apoyado en el linaje
> académico público que ya es citable.

## El problema no es un bug, es estructural

Un LLM produce afirmaciones fluidas, seguras y falsas —indistinguibles sintácticamente de
las verdaderas— y, en el otro eje, puede llevar a un humano de a poco hacia algo que no
quería. Las mitigaciones habituales (más datos, RAG, mejores prompts, modelos más grandes,
cadenas de razonamiento) **reducen** la tasa pero no la **eliminan**, y hay razones
formales para pensar que no pueden hacerlo en principio (Kalai, Nachum, Vempala & Zhang,
2025; Xu, Jain & Kankanhalli, 2024).

La causa es estructural: la categoría que volvería imposible la alucinación —el
reconocimiento explícito, calibrado y contextual del propio límite, el **"no sé" operativo**—
no puede emerger *desde adentro* de un sistema cerrado sobre su propia coherencia interna.
Un sistema cuyo único criterio de validación es la consistencia consigo mismo no tiene, por
construcción, forma de marcar "esto cae fuera de mi dominio de validez": eso exige
comparación con un referencial externo.

## La tesis: alteridad arquitectónica

> La marca del límite epistémico debe ser **introducida por un metanivel arquitectónicamente
> distinto** del sistema que genera. No se le pide al modelo que reconozca lo que no sabe:
> se construye el entorno donde ese reconocimiento es estructuralmente forzado.

No toda "segunda opinión" sirve. Hay grados de alteridad:

- **Fuerte** — un verificador cuyo *modo de falla es categóricamente distinto* y cuya
  corrección es certificable de forma independiente: un test ejecutable, un compilador, un
  `grep`, un verificador formal (Lean/Coq), una fuente leída del mundo.
- **Intermedia** — sistemas con modos de falla parcialmente independientes: neuro-simbólicos
  con verificadores externos, herramientas con ejecutores acotados.
- **Débil** — arreglos que verifican *dentro de la coherencia interna conjunta*: un LLM
  juzgando a otro LLM del mismo entrenamiento, ensembles del mismo corpus, auto-crítica
  recursiva. Apilar coherencia sobre coherencia no produce metanivel; produce coherencia
  más grande.

Esto no es una intuición sobre IA: es un principio de ingeniería con cuatro décadas de
respaldo. El *ambiguity decomposition* de Krogh & Vedelsby (1995) muestra que un ensemble
solo mejora en proporción al **desacuerdo genuino** entre sus miembros; el *N-version
programming* de Avizienis (1985) y la redundancia disímil de la normativa aeronáutica
(DO-178C) exigen **diversidad de diseño** porque las copias idénticas comparten modos de
falla; la *defense-in-depth* nuclear organiza la seguridad en capas basadas en principios
físicos distintos. Y el precedente operativo más limpio es **LavaRand**: una computadora
determinística no puede generar azar genuino desde adentro, así que se **importa entropía
física** del mundo. COGO hace lo mismo, pero con el límite epistémico en vez del azar.

## Cómo COGO lo encarna

COGO es, deliberadamente, el **dispositivo de metanivel**: no intenta que el modelo se
calibre solo (cosa que la teoría sugiere imposible), sino que produce **desde afuera** la
marca del límite, y la recomputa en cada inferencia.

| Pieza de COGO | Qué alteridad aporta |
|---|---|
| **El test ejecutable** (motor de veracidad) | Alteridad **fuerte**: leer el código, correr el comando, buscar la fuente. El color verde solo lo dicta un test que pasó. |
| **El log inmutable** (recibos de Guard) | Alteridad **fuerte** sobre la *honestidad*: el proxy tiene la transcripción, así que el gaslighting se refuta con las dos citas lado a lado. |
| **El humano** (mandato, `verify`, preguntas críticas) | El metanivel que aporta lo que el modelo no puede: juicio bajo stakes, verificación contra la realidad, sospecha legítima — y queda **registrado** en el estado de la nota. |
| **Los tiers de modelo** | Solo **proponen**, capados en amarillo, y toda propuesta se verifica contra el texto literal o se descarta. Nunca dictan el veredicto. |

De ahí sale la **regla de hierro** que comparten [`motor-veracidad.md`](motor-veracidad.md)
y [`motor-autonomia.md`](motor-autonomia.md):

> **Ningún modelo decide "es verdad" ni "esto es manipulación".** La marca sale de un test
> ejecutable, de los recibos, o del humano. Un LLM juzgando a otro LLM = **alteridad débil**
> = teatro, prohibido como oráculo final.

Y de ahí salen también los tres principios operativos:

1. **Verificación estructural, no exhortativa** — la evidencia está en el espacio operativo
   en el momento de responder, no se le pide al modelo que "verifique después". En COGO: la
   evidencia define el techo del color y el `pack` **degrada físicamente el rojo**.
2. **Paso a paso, con output observable** — una hipótesis por vez, contra el control externo
   en cada movimiento (Popper). En COGO: el pipeline por afirmación atómica.
3. **Justificación obligatoria de toda decisión categórica** — no "severidad alta" sino
   "severidad alta *porque (a), (b), (c) según el estándar X*". En COGO: cada color trae su
   `color_reason` — *no se opina, se computa*.

Un matiz clave (el del *human-in-the-loop*): la mera firma humana **no** es metanivel, es
ratificación. Para que el humano cuente como alteridad tiene que **ejercerla**: leer la
fuente, verificar un dato, juzgar un criterio, y dejarlo registrado. Por eso en COGO
`verify` cambia el color de la nota — la verificación es parte del output, no un sello.

## Las cuatro tradiciones que convergen

El diagnóstico no es nuevo ni exclusivo de la IA. Cuatro tradiciones independientes, en
lenguajes y épocas distintas, describen el mismo fenómeno estructural:

| Tradición | Núcleo | En COGO |
|---|---|---|
| **Frankfurt**, *On Bullshit* (1986) | discurso indiferente al valor de verdad | `rhetoric.bullshit` + el eje de veracidad |
| **Popper** (1959) | sin asimetría confirmación/refutación no hay conocimiento | la etapa "falsabilidad + test" |
| **Knight–Keynes–Shackle** | la incertidumbre genuina (*unknowledge*) no es probabilizable desde adentro | la abstención honesta: "no testeable acá → no verde" |
| **Kalai et al.** (2025) | el training premia adivinar y castiga "no sé" | por eso la marca viene de afuera, no del modelo |

El nombre mismo —*ingeniería del desconocimiento*— se inscribe en el linaje de la
**ingeniería del conocimiento** de Feigenbaum (1977): si aquella aportaba a la máquina el
saber experto que no tenía, esta aporta el **límite** que la máquina no puede aportarse a sí
misma.

## Qué NO es (y hasta dónde llega)

COGO **no** es cuantificación de incertidumbre (UQ), ni RLHF, ni *abstention learning*, ni
detección de *out-of-distribution*. Esas técnicas operan *dentro* de la coherencia del
modelo; COGO opera en el **plano de la arquitectura de verificación** y produce una marca
que permanece estructuralmente externa en cada inferencia.

Y una honestidad de alcance: COGO tiene alteridad fuerte solo para la rebanada
**fáctica/ejecutable** del problema (un dato, una cita, una sintaxis: el ejecutor lo
confirma o no). Para el desconocimiento *estructural* —inventar el criterio, el marco, la
taxonomía correcta— COGO **no** tiene metanivel, y lo correcto es que **se abstenga** (no
finge un verde) en vez de simular un juicio que no puede sostener.

---

### Referencias (linaje público)

- Avizienis, A. (1985). *The N-Version Approach to Fault-Tolerant Software.*
- Feigenbaum, E. (1977). *The Art of Artificial Intelligence.*
- Frankfurt, H. (1986). *On Bullshit.*
- Kalai, A., Nachum, O., Vempala, S. & Zhang, E. (2025). *Why Language Models Hallucinate.*
- Kalai, A. & Vempala, S. (2024). *Calibrated Language Models Must Hallucinate* (STOC).
- Knight, F. (1921). *Risk, Uncertainty and Profit.*
- Krogh, A. & Vedelsby, J. (1995). *Neural Network Ensembles, Cross Validation, and Active Learning.*
- Popper, K. (1959). *The Logic of Scientific Discovery.*
- Shackle, G. L. S. (1972). *Epistemics and Economics.*
- Xu, Z., Jain, S. & Kankanhalli, M. (2024). *Hallucination is Inevitable.*

*Marco integrador y bibliografía completa: working paper en preparación (no publicado aún).*
