# Estado único de resumen y evento summary_resolved

## Contexto

El resumen es opcional: el cliente lo pide o no lo pide en la misma solicitud que manda el documento. Eso, que
parece un detalle, crea un problema de vocabulario que hay que resolver antes de escribir una sola línea.

El problema: un documento puede tener el campo de resumen vacío por **tres razones distintas**, y el sistema
tiene que poder distinguirlas.

1. El cliente lo pidió y el Resumidor no respondió. Falló.
2. El cliente no lo pidió. Nunca se intentó.
3. El cliente lo pidió, respondió, y el evento todavía está en el stream. Está en camino.

Los tres casos se ven iguales si solo tenés un campo `summary` que es `null`. Y esa indistinción no es cosmética:
**el cliente necesita saber si reintentar.** En el caso 2 no hay nada que reintentar porque nunca se pidió. En el
caso 3 va a llegar solo y consultando de nuevo tiene sentido. En el caso 1 hay que volver a pedirlo.

El consumidor del stream es el que tiene el dato persistido, así que la tentación es que infiera el estado de
resumen a partir de si el campo está lleno o no. Pero el consumidor **no sabe si el cliente lo pidió**: solo ve
que no llegó un evento de resumen. Eso no distingue el caso 1 del caso 2, porque en los dos el campo está vacío
y no hay evento.

## Decisión

**El estado de resumen lo decide el Orquestador y viaja explícito. El consumidor no lo infiere, no lo calcula y no
lo deduce.**

Como consecuencia directa, el evento que lleva el resumen se llama **`summary_resolved`**, no `summary`. El
nombre importa: `summary` describe un dato, `summary_resolved` describe un hecho — que alguien resolvió el
resumen de este documento. El sufijo es lo que le dice al consumidor que este evento resuelve una pregunta y no
que es simplemente parte de un registro.

El estado de resumen es un eje propio, con valores que **solo el Orquestador puede conocer**:

| Valor | Significado | Quién lo escribe |
|---|---|---|
| `NOT_REQUESTED` | el cliente pidió el documento sin resumen | el Orquestador, al admitir la ingesta |
| `PENDING` | se pidió y se está resolviendo | el Orquestador |
| `RESOLVED` | se resolvió y se publicó | el Orquestador |
| `FAILED` | se pidió y el Resumidor no respondió | el Orquestador |

Y de la misma forma, el estado de entrega es un eje separado con `INGESTED` y `AVAILABLE`. Los dos ejes son
independientes y no se derivan uno del otro. Un documento puede estar `INGESTED` con el resumen en cualquiera de
los cuatro valores, y un documento `AVAILABLE` puede quedarse en `NOT_REQUESTED` para siempre. La API expone los
dos por separado precisamente para que el cliente no tenga que inferir nada.

### "Estado único" significa fuente única, no un solo estado

El nombre de este ADR dice "estado único" y conviene dejar explícito qué significa, porque es ambiguo y la
ambigüedad ya causó una contradicción entre repositorios.

**"Estado único de resumen" significa que hay una sola fuente de ese valor: el Orquestador.** No significa que
exista un solo estado ni que los dos ejes se fundan en uno.

El Persistidor guarda internamente un `Status` único de dos valores, `PENDING` y `COMPLETED`
(`ARCHITECTURE_REPOSITORIES.md` §8). Eso no contradice esta decisión: es una representación de almacenamiento
que responde a otra pregunta —si el documento tiene su resumen presente o no— para uso interno del consumidor.
El vocabulario de la API pública son los dos ejes de cuatro y dos valores. Son dos preguntas distintas, con dos
consumidoras distintas.

## Alternativas consideradas

**Evento `summary`, como está hoy en el blueprint del Persistidor.** Rechazada. Un evento que solo lleva un
dato no dice nada sobre el caso 1 frente al caso 2: el consumidor ve que el campo está vacío y no puede saber si
falló o si nunca se pidió. Nombrarlo `summary_resolved` obliga a que exista un evento para cada resumen resuelto,
que es lo que hace que los tres casos sean distinguibles desde el stream.

**Que el Persistidor deduzca el estado de resumen del campo `summary`.** Rechazada por la razón de fondo del
problema: el consumidor no tiene la información. No sabe si el cliente lo pidió. Cualquier heurística que
intente —"si está vacío y pasó X tiempo, es que falló"— convierte un dato desconocido en una suposición, y las
suposiciones sobre datos ausentes producen documentos que parecen completos y no lo están.

**Colapsar los dos ejes en un solo `Status`.** Rechazada. Fusionar "está persistido" con "tiene resumen"
obliga a que el cliente infiera: un `PENDING` no dice si el resumen no se pidió o si el documento todavía no
llegó, y esas dos cosas piden acciones opuestas. Son dos preguntas, y la API responde las dos.

**Que el Orquestador consulte al Persistidor para saber el estado de resumen.** Rechazada. El Orquestador es el
que decide el estado de resumen, así que consultarlo sería preguntarle a quien lo calculó qué calculó. Además
mete una lectura sincrónica a la base en el camino de una respuesta que no debería depender de ella.

## Consecuencias

### Positivas

- **El cliente puede distinguir los tres casos** y sabe si tiene sentido reintentar. `NOT_REQUESTED` no se
  reintenta, `FAILED` sí, `PENDING` se espera.
- **El consumidor no deduce nada.** La distinción entre fallo y ausencia está en los datos, no en una heurística
  del consumidor.
- **El nombre del evento documenta su propio propósito.** `summary_resolved` se lee solo.
- **La API expone los dos ejes y el cliente no tiene que interpretar la ausencia de un campo.**

### Negativas / costo

- **Es un cambio de contrato para el Persistidor.** El blueprint actual llama `summary` al evento en
  `ARCHITECTURE_REPOSITORIES.md` §4 y §9, y esa referencia tiene que cambiar. Es la única ruptura del contrato
  cross-repo y está detallada en `docs/contracts/delta-persister.md`. Cambiarlo después, con mensajes en vuelo o
  con un consumidor desplegado, sería mucho más caro que cambiarlo ahora.
- **Más vocabulario en la API.** Dos ejes y seis valores en total, contra un `Status` de dos. Es más superficie de
  contrato que mantener, a cambio de que el cliente no adivine.
- **El Orquestador es responsable de mantener el estado de resumen coherente** en las dos rutas que lo tocan: la
  ingesta y el endpoint de resumen a demanda (ADR-0008). Que el segundo camino también escriba este valor es
  una consecuencia de que la fuente sea única.