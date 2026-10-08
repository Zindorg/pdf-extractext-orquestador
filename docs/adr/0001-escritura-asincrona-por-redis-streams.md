# Escritura asíncrona por Redis Streams

## Contexto

El Persistidor es el único servicio con acceso a la base de datos, y es el dueño del ciclo de vida del
documento. Pero la regla del ecosistema es hub & spoke: los workers no se hablan entre sí, y todo pasa por
el Orquestador. De ahí sale una restricción que hay que resolver antes de escribir una línea de la API: si el
Orquestador no puede llamar al Persistidor para escribir, y el Persistidor no puede aceptar conexiones de
nadie más, **la escritura tiene que viajar por un canal que no sea HTTP entre servicios**.

Ese canal tiene que cumplir cuatro cosas a la vez. Tiene que tolerar que el consumidor esté caído sin que se
pierda el documento, porque un documento que se pierde en silencio es peor que uno que responde con error. Tiene
que poder entregar el mismo documento **dos veces** sin romper nada, porque la semántica de reintento de Redis
es *at-least-once* y no hay forma de cambiarla desde el productor. Tiene que dejar un rastro cuando el
consumidor rechaza un mensaje, para que exista una cola de rechazados y no un agujero. Y tiene que ser
operable por el equipo que mantiene el Persistidor sin tocar el Orquestador.

La tentación obvia —que el Orquestador le haga un `POST /documents` al Persistidor— viola la segunda
restricción en la forma más cara posible: convierte una base de datos detrás de una cola en un servicio HTTP
que cualquiera de la red interna puede invocar, y pone la latencia de MongoDB en el camino crítico del cliente.

## Decisión

**La escritura viaja por Redis Streams. El Orquestador publica; el Persistidor consume. El cliente recibe
`202 Accepted`.**

El stream es `document-events` y el grupo de consumidores es `documents-persister`. El Orquestador es su único
productor: usa `XADD` y nada más — no lee, no consume, no mantiene grupo (`docs/contracts/events.md`).

La ingesta tiene dos fases por documento. El evento `original` lleva el texto extraído, su checksum y su
metadato; el evento `summary_resolved` lleva el resumen. El Persistidor inserta `PENDING` con el primero y
completa con el segundo. Si el cliente pidió `summarize=false`, el segundo evento simplemente no existe.

**El `XADD` va al final del pipeline, después de que el resumen resolvió.** Es la parte de esta decisión que
no es obvia y que conviene explicitar:

```
1. recibir      multipart en RAM, generar document_id (UUID v4)
2. puerta 1     tamaño · tipo · cifrado                        → 413 / 415 / 400
3. extraer      Extractor por HTTP                            → mide extraction_time_ms
4. puerta 2     clasificar por motivo                          → 422, nada se ingiere
5. checksum     sha256(texto extraído) en el productor
6. resumir      solo si summarize=true                         → mide summary_time_ms
                                                          falla → 502, nada se ingiere
7. publicar     XADD original, luego XADD summary_resolved
8. responder    202 + Location + processing_time_ms (= 3 + 6)
```

Publicar antes de resumir produciría un estado que no tiene dueño. Si el Resumidor falla en el paso 6 con el
documento ya publicado, el texto queda en la base ocupando su checksum, y el cliente recibió un error y nunca
supo su `document_id`: un documento invisible que además bloquea el reenvío de ese mismo contenido. Publicando
al final, o se guardó todo y se avisó éxito, o no se guardó nada. No hay estado intermedio.

El precio de esa elección es explícito y aceptado: **una caída del Resumidor devuelve `502` en las peticiones
con `summarize=true` y descarta una extracción que sí funcionó.** El radio de impacto queda acotado a los
clientes que piden resumen, y el camino de recuperación es reintentar con `summarize=false`, que funciona
completo. Un resumen perdido se recupera; un documento invisible con su checksum ocupado, no.

## Alternativas consideradas

**Alta REST desde el Orquestador (`POST /documents`).** Rechazada. Rompe el hub & spoke en el único punto
donde el aislamiento importa más, expone un endpoint de escritura en la red interna que el resto de los
servicios podrían invocar, y pone la latencia de MongoDB y la indisponibilidad de la base en el camino crítico
del cliente. Además obligaría al Persistidor a decidir qué hacer con un `document_id` duplicado en el camino
HTTP, que es la misma decisión que hoy toma el índice único dentro del consumidor.

**El Orquestador como consumidor y el Persistidor como RPC puro.** Rechazada. Invierte el flujo: el
documento pasaría por el proceso del Orquestador, que es el más expuesto y el que más se escala hacia afuera.
La tolerancia a fallos quedaría en el camino caliente en vez de en el lado del consumidor.

**NATS, Kafka o similar.** Rechazada por costo de infraestructura. Hoy hay un solo productor y un solo
consumidor; agregar un broker con particionamiento, offsets y grupos antes de que exista una necesidad real es
pagar complejidad por un problema que Redis Streams ya resuelve con dos operaciones.

**Escritura síncrona esperando confirmación del Persistidor.** Rechazada. No mejora la restricción —el
documento igual tiene que persistirse— pero sí acopla el tiempo de respuesta del cliente a la disponibilidad de
la base, y elimina la propiedad que hace que el 202 sea barato.

## Consecuencias

### Positivas

- El cliente no espera a la base. El tiempo de respuesta depende del Extractor y del Resumidor, no de MongoDB.
- La tolerancia a fallos vive del lado del consumidor: reintentos, espera creciente y cola de rechazados están
  en el Persistidor, que es quien puede reintentarlos sin que nadie los espere.
- La duplicación es barata y explícita: el productor publica sin coordinación con el consumidor, y el consumidor
  aplica la regla de duplicados en un solo lugar.
- El Persistidor no expone escritura por HTTP. Su superficie interna es de solo lectura y borrado.
- El checksum lo calcula una sola vez, en el productor, y viaja tal cual (ver ADR-0003).

### Negativas / costo

- **El cliente tiene que consultar después.** El 202 significa "la ingesta está en camino", no "el documento
  está disponible". Aparece el caso *aún ingerido*, que es un 404 transitorio y necesita su propio código de
  error para no confundirse con *no existe*.
- **Un `original` perdido en el stream no lo nota nadie.** A diferencia de un error HTTP, que el cliente ve
  inmediatamente, un mensaje que se pierde entre el productor y el consumidor deja el cliente con un `202` y
  un documento que nunca existió. Es el riesgo grande de la escritura asíncrona y es real.
- **El hueco del documento invisible.** Si el proceso muere entre los dos `XADD`, el documento queda ingestado
  sin resumen y el cliente nunca recibe el `document_id`. Al reenviar, el Persistidor descarta por checksum
  duplicado y el cliente obtiene un `202` de un `document_id` que no existe. La ventana es de milisegundos y
  cualquier flujo publicar-luego-acknowledger la tiene, pero no la tiene gratis.
  - *Mitigación inmediata:* el contrato de lectura distingue el 404 transitorio del terminal mediante una regla
    con plazo, lo que corta el bucle de reintentos del cliente
    (`docs/contracts/openapi.yaml`).
  - *Curación de fondo:* un reconciliador que detecte y relance los documentos sin resumen. Contradice que el
    Orquestador sea solo productor del stream, así que es un componente nuevo, no una decisión local.
- **Tolerancia a fallos que hay que operar.** El stream, su grupo de consumidores y la cola de rechazados son
  infraestructura que alguien tiene que mirar.
- **Nada se ingiere hasta el final.** Es una decisión deliberada y tiene el costo descrito arriba: una caída
  del Resumidor tira una extracción buena.