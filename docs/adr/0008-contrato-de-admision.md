# Contrato de admisión

## Contexto

El endpoint de admisión tiene tres responsabilidades que se pueden decidir de tres maneras distintas, y las
tres son escolhas, no detalles:

**Qué devuelve.** Puede devolver `200` con el documento ya persistido, o `202` con una promesa. La diferencia es
si la respuesta espera al trabajo asíncrono.

**Cuándo publica.** Puede publicar el texto extraído apenas sale del Extractor, o esperar a que termine también
el resumen.

**Qué pasa si el resumen falla.** Puede devolver un `200` y avisar que el resumen no está, o fallar la ingesta
entera.

Estas tres preguntas se responden juntas, y contestarlas por separado produce combinaciones que no tienen
sentido. Esta es la primera de las tres: qué devuelve y cuándo.

El punto de partida es que el blueprint original devuelve `200 OK` con los campos de la respuesta y **espera a
que el documento esté persistido**, lo que hace que la escritura asíncrona de ADR-0001 y la respuesta síncrona
se contradigan.

## Decisión

**El endpoint devuelve `202 Accepted` con el `document_id` en el cuerpo, en el header `X-Document-Id` y en un
header `Location`. No espera a que el documento sea consultable.**

Tres consecuencias que se derivan y que conviene no separar:

- **El estado de entrega inicial es `INGESTED`**, no `AVAILABLE`. `INGESTED` significa publicado en el stream y
  pendiente de que el Persistidor lo asiente. El cliente sabe que el texto ya está en camino pero que la lectura
  puede dar `404` todavía.
- **`AVAILABLE` no lo decide el Orquestador**, sino el hecho de que el documento exista en el Persistidor. No hay
  ningún endpoint que "avise" de que un documento quedó disponible.
- **El `Location` es el punto de reintento del cliente.** Apunta a la ruta de lectura del documento en el
  Persistidor, que es donde el cliente va a consultar el resultado.

## Alternativas consideradas

**Devolver `200 OK` esperando a la persistencia.** Rechazada. Es la opción del blueprint original y contradice
ADR-0001: si el consumidor del stream falla, reintenta con espera creciente, y el cliente ya recibió un `200`
que dice que todo salió bien. El cliente no tiene forma de distinguir "persistido" de "publicado y todavía en
vuelo". También hace que la latencia de la respuesta sea la de la base de datos del Persistidor, no la del
pipeline.

**Devolver `200 OK` sin esperar, igual que `202`.** Rechazada. El código `202` dice algo que `200` no dice:
que hay trabajo pendiente. Un cliente que ignore la diferencia no tiene forma de saberlo, y la distinción entre
"listo" y "en camino" es justamente la que necesita.

**Devolver `202` con un identificador de trabajo en vez de con el `document_id`.** Rechazada porque serían dos
identificadores, y ADR-0004 deja un solo identificador en el sistema. Además el identificador de trabajo
tendría que ser consultable, lo que agrega un endpoint y un estado más.

## Consecuencias

### Positivas

- **El cliente sabe distinguir las dos fases.** Con `202` y `INGESTED` sabe que tiene que consultar; con `200` y
  `AVAILABLE` sabría que no hace falta.
- **La respuesta no depende del Persistidor.** Si el Persistidor está lento o caído, la admisión responde igual,
  y el texto queda en el stream esperando a que vuelva.
- **La latencia de la respuesta es la del pipeline**, no la de la base de datos del consumidor.
- **El `document_id` se conoce al instante**, y sirve para el header, para las rutas y para correlacionar logs.

### Negativas / costo

- **El cliente tiene que consultar.** Devolver el `document_id` no devuelve el documento, y el cliente necesita
  un segundo paso para leerlo. Es el precio de que la escritura sea asíncrona.
- **El `404` inmediato hay que explicarlo.** Si el cliente consulta de inmediato y el Persistidor todavía no
  asintió, recibe `404` aunque la ingesta haya sido aceptada. Un cliente bien escrito reintenta; uno mal escrito
  lo trata como error permanente.
- **El header `Location` apunta a otro servicio**, así que un cliente que siga el `Location` tiene que saber
  que la lectura vive en el Persistidor, no en el Orquestador.
