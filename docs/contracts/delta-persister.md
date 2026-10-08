# Delta para el Persistidor

> **Para revisar por el equipo del Persistidor.** Este documento lista, una por una, las diferencias entre lo
> que `ARCHITECTURE_REPOSITORIES.md` describe hoy y los contratos que el Orquestador cerró. Cada fila indica qué
> cambia, si **rompe** al Persistidor o es **aditivo** / **cosmético**, qué ADR o contrato lo origina, y el lugar
> exacto a tocar en el blueprint.
>
> **Condición bloqueante:** la implementación del Orquestador no arranca hasta que este documento sea
> reconocido por el equipo del Persistidor. Sin ese reconocimiento los dos repos divergen y el test de
> integración falla.

## Lo principal

**Hay **una sola ruptura**: el evento `summary` se renombra a `summary_resolved`.** Todo lo demás es agregar
campos, cambiar un nombre de red o renombrar un header; nada vuelve a interpretar los que ya existen.

## Tabla de cambios

| # | Qué cambia | Tipo | Origen | Dónde tocar en `ARCHITECTURE_REPOSITORIES.md` |
|---|---|---|---|---|
| 1 | El evento de resumen se llama `summary_resolved`, no `summary` | **rompe** | ADR-0006 · `docs/contracts/events.md` | §4 (tabla de estados y flechitas), §7 (handlers), §9 (mensaje `summary`) y §Table de consumibles |
| 2 | `processing_time_ms` **no viaja en los eventos**: el `original` lleva `extraction_time_ms` y el `summary_resolved` lleva `summary_time_ms`. El `processing_time_ms` total (= suma) solo existe en la respuesta HTTP de la ingesta, no en el stream | **aditivo + rompe** * | ADR-0001 · `events.md` §Tiempos · `openapi.yaml` `ExtractAccepted` | §9 mensaje `original` (quitar `processing_time_ms`, sumar `extraction_time_ms`) y mensaje `summary` → `summary_resolved` (agregar `summary_time_ms`) |
| 3 | Cada mensaje lleva `schema_version` (entero, hoy 1) | **aditivo** | `events.md` (ambos eventos) | §9, ambos mensajes |
| 4 | El evento lleva `event_type` además del nombre del mensaje | **aditivo** | `events.md` | §9, ambos mensajes |
| 5 | El `original` lleva `summary_requested` y `summary_status`; el `summary_resolved` lleva `summary_status=RESOLVED` | **aditivo** | ADR-0006 · `events.md` | §9, ambos mensajes |
| 6 | El `original` lleva `mime_type` como **campo del evento**, no como parte de `metadata`, y el checksum es del **texto extraído** (`sha256`, minúsculas) | **aditivo** | ADR-0003 · ADR-0008 · `events.md` | §9 mensaje `original` |
| 7 | Header de correlación: `X-Request-ID` → `X-Document-Id` (el identificador es del documento, ver ADR-0004) | **cosmético** (el header no participa de la lógica) | ADR-0004 · `openapi.yaml` | §10 envelope de error |
| 8 | Red interna: `internal-net` → `mired` | **cosmético** | ADR-0005 · `ARCHITECTURE_ORCHESTRATOR.md` §7 | §11 despliegue, §13 compose |
| 9 | El contenedor del Orquestador conecta a `mired` + `public-net`; el Persistidor a `mired` + `db-net` | **cosmético** si se renombra en red (no cambia topología) | ADR-0005 | §13 compose |
| 10 | Prefijo de rutas: la API interna sigue montada `/api/v1`, **sin reescritura** por el proxy (*strip-prefix off*) | ver contradicción §10 vs §13 abajo | ADR-0005 · `openapi.yaml` | §10 tabla y §13 healthcheck |
| 11 | Nueva ruta pública del Orquestador `POST /api/v1/documents/{document_id}/summary` | **aditivo (cero cambios en este repo)** | ADR-0006 · `openapi.yaml` §summary | ninguno: usa GET `/documents/{id}` y GET `/download/original` de §10 |
| 12 | `409` en `GET /download/summary` cuando el resumen está `PENDING` ya está en §10; se confirma y se alinea el `code` a `summary_pending` del Orquestador | **cosmético** | `openapi.yaml` ErrorCode | §10, fila de download/summary |

\* El ítem 2 es rompe y aditivo a la vez: *rompe* porque el mensaje `original` cambia de shape (sale
`processing_time_ms`, entra `extraction_time_ms`) y requiere migrar el consumidor al nuevo nombre; *aditivo*
porque no cambia la semántica de los campos que ya existían.

## Sobre la contradicción §10 vs §13 (prefijo)

`ARCHITECTURE_REPOSITORIES.md` se contradice sobre el prefijo de la API:

- **§10** titula su tabla *"Contrato HTTP interno `/api/v1`"* y luego lista rutas **sin** el prefijo:
  `GET /documents/{document_id}`, `POST /documents/{document_id}/restore`, etc.
- **§13** usa el prefijo en el healthcheck: `wget http://localhost:8083/api/v1/health`.

**Qué se corrige:** los endpoints internos viven bajo `/api/v1` (§10). La *tabla* de §10 debería listar las
rutas con el prefijo, para que ambas secciones coincidan y para que la allowlist del proxy del Orquestador
(`internal/api/proxy.go`) se escriba contra el mismo string que el router del Persistidor. La regla de
transparencia es: el proxy **no reescribe** —el prefijo `/api/v1` se conserva tal cual (ADR-0005)—.

## Lo que NO cambia en el Persistidor

- La escritura sigue siendo **evento-dirigida** por stream: no entra ningún `POST /documents` (§10, nota de
  diseño).
- La deduplicación por `checksum` activo sigue viviendo en el índice único parcial de Mongo (§8). El checksum
  que ahora se calcula del **texto extraído** (no del archivo) sigue siendo la identidad del contenido.
- El `Status` interno de dos valores (`PENDING` / `COMPLETED`) **no cambia**: es la representación de
  almacenamiento del eje `summary_status` (ADR-0006, glosario `CONTEXT.md`), y sigue siendo correcto que el
  documento este `COMPLETED` cuando llegó su `summary_resolved`.
- El `409` al restaurear con checksum ocupado, la paginación y el envelope de error se mantienen.

## Qué espera el Orquestador del Persistidor al implementar

1. Que consuma el evento `summary_resolved` igual que consumía `summary` (update por `document_id` →
   `COMPLETED`, ACK + descarte si ya está completo).
2. Que los `schema_version` desconocidos se traten como mensaje inválido (retry → DLQ), no como éxito.
3. Que `extraction_time_ms` y `summary_time_ms` se guarden en su registro —el Orquestador los manda porque son
   los originales de cada pata, y `processing_time_ms` público (la suma) se calcula en la API de lectura si se
   quiere exponer—.
4. Que el `409` de "resumen aún `PENDING`" en `GET /download/summary` siga existiendo; el Orquestador lo
   traduce a su propio `code: summary_pending`.