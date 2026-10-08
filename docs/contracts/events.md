# Contrato del stream de ingesta

Este es el contrato del stream visto desde el productor. La perspectiva del consumidor —grupo, reintentos,
cola de mensajes rechazados— está en `ARCHITECTURE_REPOSITORIES.md` §9 y no se duplica acá.

## Convenciones

| Elemento | Valor |
|---|---|
| Stream | `document-events` |
| Productor | Orquestador, único |
| Consumidores | Persistidor, grupo `documents-persister` |
| Transporte | HTTP/1.1 + JSON, no Protobuf (ADR-0002) |
| Clave del mensaje | `document_id`, para que dos eventos del mismo documento no se intercalen |

Un solo productor es una decisión, no una casualidad: hace imposible la condición de carrera entre dos
escritores sobre el mismo documento, que es lo que obliga al consumidor a reintentar con espera creciente.

### Orden de publicación

El orden importa y es el siguiente, siempre:

1. `XADD original` — **después** de que el resumen resolvió, o de que se decidió no pedirlo.
2. `XADD summary_resolved` — solo si `summarize=true`.

Lo segundo que hay que entender es que **`original` no se publica cuando termina la extracción.** Se publica
cuando termina todo el pipeline. Es la consecuencia directa de que un fallo del Resumidor no deje nada
ingerido: si `original` saliera al stream en el paso 4, el documento ya estaría persistido cuando el Resumidor
fallara, y un `502` dejaría un documento que el cliente cree que no existe.

Esa es también la razón de que la respuesta `502` sea limpia: si nada se publicó, no hay nada que deshacer.

## Versionado

Todo mensaje lleva `schema_version`, un entero que empieza en `1`. Va dentro del payload, no en un header de
Redis, para que un mensaje sea legible sin conocer nada del transporte.

Reglas de compatibilidad:

- **El consumidor ignora los campos desconocidos.** Es obligatorio: sin esta regla, agregar un campo es una
  ruptura.
- **El productor nunca cambia el significado de un campo existente.** Se agrega un campo nuevo, o se sube
  `schema_version` y se publica un contrato nuevo.
- **Quitar un campo o cambiarle el tipo es una ruptura** y exige subir `schema_version` y avisar al equipo del
  Persistidor antes, no después.

## Evento `original`

Transporta el texto extraído de un documento y lo hace consultable. Es el evento que crea el documento en el
sistema.

```json
{
  "schema_version": 1,
  "event_type": "original",
  "document_id": "0f8fad5b-d9cb-469f-a165-70867728950e",
  "checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "extracted_text": "texto plano del documento",
  "mime_type": "application/pdf",
  "summary_requested": true,
  "summary_status": "RESOLVED",
  "extraction_time_ms": 1240,
  "extracted_at": "2026-09-30T14:22:05Z"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `schema_version` | entero | sí | Versión del esquema del payload. Hoy, siempre `1`. |
| `event_type` | string | sí | `"original"`. Permite al consumidor distinguir por tipo y no por el nombre del stream. |
| `document_id` | string (UUID v4) | sí | Identificador de la ingesta (ADR-0004). Es también la clave del mensaje. |
| `checksum` | string (64 hex) | sí | `sha256` del texto extraído, en minúsculas (ADR-0003). |
| `extracted_text` | string | sí | El texto extraído. **Es el contenido**: el archivo original no viaja y no se guarda en ningún lado. |
| `mime_type` | string | sí | Tipo del documento recibido, tal como lo declara el cliente. |
| `summary_requested` | booleano | sí | Si el cliente pidió resumen **en esta ingesta**. |
| `summary_status` | string | sí | Estado de resumen en el momento de publicar: `RESOLVED` o `NOT_REQUESTED`. Los otros dos valores no pueden aparecer acá, porque este evento se publica cuando el resumen ya resolvió o nunca se pidió. |
| `extraction_time_ms` | entero | sí | Duración de la extracción **en milisegundos**. Solo esta pata, no el total. |
| `extracted_at` | string (RFC 3339) | sí | Momento en que terminó la extracción, en UTC. |

`summary_status` es el campo que le evita al consumidor tener que adivinar entre "falló" y "no se pidió"
(ADR-0006).

`summary_requested` parece redundante con `summary_status` —de hecho lo es en el camino normal, porque
`NOT_REQUESTED` implica `false` y los otros tres valores implican `true`— pero **no lo es en un caso**: después de
que el endpoint de resumen a demanda publique un `summary_resolved`, el documento va a tener resumen con un
`summary_requested` de `false` en su evento `original`. El par `summary_requested=false` con
`summary_status=RESOLVED` es un estado legítimo y el consumidor tiene que poder representarlo. Por eso el
consumidor guarda el estado de resumen **copiando** el valor del evento, y no deduciéndolo del par.

### Sobre `extracted_text` vacío

Un documento cuyo texto sale vacío **no se publica**: la puerta 2 lo rechaza con `422`. Un evento `original`
siempre lleva texto.

### Sobre el tamaño de los campos del evento

**No hay un techo explícito para `extracted_text` ni `summary`.** La puerta 1 topea el archivo en 10 MiB antes
de llamar al Extractor, y el texto extraído de un PDF no puede superar el contenido del propio archivo (no hay
OCR en el MVP; fuentes, imágenes y estructura ocupan espacio que no es texto), así que queda acotado por el
mismo techo del archivo sin un límite adicional. El `summary` es síntesis en lenguaje natural, sustancialmente
menor que el texto. Redis Streams acepta mensajes hasta `proto-max-bulk-len` (512 MiB por defecto), muy por
encima de ese tope. Si algún día aparecen formatos que expandan el texto más allá del archivo, se define un
límite explícito en este contrato; hoy no hay caso que lo motive.

## Evento `summary_resolved`

Transporta el resumen ya calculado de un documento cuyo texto ya fue publicado. Es el segundo evento del ciclo de
vida y es **condicional**: solo existe si el cliente pidió resumen.

```json
{
  "schema_version": 1,
  "event_type": "summary_resolved",
  "document_id": "0f8fad5b-d9cb-469f-a165-70867728950e",
  "checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "summary": "síntesis en lenguaje natural del texto extraído",
  "summary_status": "RESOLVED",
  "summary_time_ms": 8300,
  "resolved_at": "2026-09-30T14:22:14Z"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `schema_version` | entero | sí | Versión del esquema del payload. |
| `event_type` | string | sí | `"summary_resolved"`. El nombre dice que alguien resolvió el resumen, no que es una parte de un registro (ADR-0006). |
| `document_id` | string (UUID v4) | sí | El documento al que pertenece. Debe existir: este evento nunca es el primero. |
| `checksum` | string (64 hex) | sí | Se repite el mismo valor que en `original`. Permite al consumidor validar coherencia sin consultar. |
| `summary` | string | sí | El resumen. No vacío: si el Resumidor no devolvió nada utilizable, la ingesta falla antes de publicar. |
| `summary_status` | string | sí | Siempre `RESOLVED` en este evento. El valor es constante porque el nombre del evento ya lo dice. |
| `summary_time_ms` | entero | sí | Duración de la llamada al Resumidor **en milisegundos**. Solo esta pata, no el total. |
| `resolved_at` | string (RFC 3339) | sí | Momento en que resolvió el resumen, en UTC. |

Este evento también lo emite el endpoint de resumen a demanda (`docs/contracts/openapi.yaml`), con el mismo
formato. El consumidor no distingue de dónde vino.

## Tiempos: cada evento lleva su propia pata

`processing_time_ms` **no viaja en ningún evento**. El contrato HTTP tampoco lo expone.

La regla es que cada evento lleva **solo** el tiempo de su etapa: `original` lleva `extraction_time_ms`,
`summary_resolved` lleva `summary_time_ms`. La suma de los dos es el tiempo total de la ingesta, y se calcula
quien lo necesite, no se transmite.

La razón de no mandar el total es que **no hay un evento que lo represente**: `original` se publica después de
que el resumen resolvió, así que cuando viaja, el total ya se conoce —y podría llevar el— pero eso haría que
un evento sepa del trabajo del otro y los acopla. `summary_resolved` no puede llevar el total en ningún caso:
si el Resumidor falla, ese evento no existe, y el total se pierde. Un campo cuyo valor existe a veces y no
otras es peor que dos campos que existen siempre.

## Reintentos, DLQ y duplicados

Lo que este servicio garantiza y lo que no:

### Garantizado

- **Publicación en dos pasos, o ninguna.** Si falla el pipeline antes de publicar, no hay ningún mensaje en el
  stream. No existen eventos a medias.
- **`summary_resolved` nunca viaja sin un `original`.** El orden de publicación lo garantiza, y el consumidor lo
  reintenta con espera creciente si lo encuentra huérfano (ver "documento abandonado" en `CONTEXT.md`).
- **El `document_id` no se reutiliza.** Un evento en el stream siempre nombra una ingesta distinta.
- **Idempotencia en el consumidor.** El consumidor debe poder recibir `original` dos veces sin duplicar el
  documento. La garantía fuerte de deduplicación está en un índice único parcial por `checksum`
  (`ARCHITECTURE_REPOSITORIES.md` §8), no en este servicio.

### No garantizado

- **No hay exactamente una entrega.** Redis Streams entrega **al menos una vez**. El consumidor tiene que
  tolerar duplicados y el productor no puede saber cuáles son.
- **No hay orden entre documentos.** El orden está garantizado dentro de un documento, porque la clave del
  mensaje es el `document_id`. Entre documentos distintos, no.
- **No hay entrega confirmada.** El `XADD` devuelve un identificador de mensaje, no un acuse de recibo del
  consumidor. El Orquestador no sabe si el Persistidor leyó el mensaje.
- **No hay compensación.** Una vez publicado, el Orquestador no borra ni modifica mensajes. Si el consumidor
  rechaza un mensaje para siempre, el documento queda ingested pero nunca disponible, y solo un reconciliador
  —que no existe en el MVP— lo arreglaría.

### Qué espera el consumidor de este servicio

1. Que `extracted_text` no llegue vacío.
2. Que `checksum` sea el `sha256` de `extracted_text` en minúsculas, y que no lo recalcule.
3. Que `summary_resolved` llegue después de `original`, para el mismo `document_id`.
4. Que ignore los campos que no conozca, y que un `schema_version` mayor no lo haga fallar.
5. Que no espere el campo `processing_time_ms`: no existe.
