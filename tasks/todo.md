# Tareas: cerrar los contratos y los ADR del Orquestador

> Plan de referencia: `tasks/plan.md`. Solo documentación. Cero lógica de código; los comentarios dentro de
> `.go` pueden tocarse cuando contradicen una decisión cerrada.
>
> Regla de los checkpoints: son momentos de juicio humano, no repeticiones del grep. El grep es la capa 1 de
> la verificación (ver `tasks/plan.md`); la capa 2 es la lectura en frío de la tarea 15.

---

## Fase 1 — Vocabulario

### Task 1: `CONTEXT.md` — las 11 definiciones

**Descripción:** el glosario fija el vocabulario del que dependen los nombres de todas las fases siguientes.
Las listas de "Evitar" de cada término son restricciones de escritura, no sugerencias.

**Criterios de aceptación:**
- [ ] Los 11 términos tienen definición: `Documento`, `Identificador de documento`, `Texto extraído`,
      `Resumen`, `Checksum`, `Duplicado`, `Estado de entrega`, `Estado de resumen`, `Ingesta`,
      `Documento abandonado`, `Zero-Disk`.
- [ ] Cada definición dice qué es **y qué no es**. Una definición que solo dice qué es no descarta el término
      equivocado.
- [ ] Ninguna definición usa como término principal ninguno de los listados en "Evitar" de su sección.
- [ ] `Estado de entrega` y `Estado de resumen` quedan definidos como ejes independientes, y queda escrito que
      el `Status` único del Persistidor (`ARCHITECTURE_REPOSITORIES.md` §8) es una representación de
      almacenamiento, no el vocabulario de la API.
- [ ] Queda escrito que "estado único de resumen" (ADR-0006) significa fuente única del valor —el
      Orquestador, que es el único que sabe si el cliente lo pidió—, no que exista un solo estado.
- [ ] Los valores posibles de cada eje quedan nombrados aquí, porque `openapi.yaml` los referencia y no puede
      inventar otros.
- [ ] `ARCHITECTURE_ORCHESTRATOR.md` recibe 2 líneas de banner al inicio: su contenido está en revisión y la
      fuente vigente son `docs/adr/` y `docs/contracts/`.
- [ ] `grep -c "_TODO_" CONTEXT.md` devuelve 0.

**Verificación:**
- [ ] Tests: no aplica, no hay código.
- [ ] Build: no aplica.
- [ ] Manual: leer el glosario entero y confirmar que ningún lector puede confundir dos términos parecidos.

**Dependencias:** ninguna. Es la primera.

**Archivos:** `CONTEXT.md`, `ARCHITECTURE_ORCHESTRATOR.md` (solo el banner)

**Alcance:** M

---

## Fase 2 — ADR

> Las tareas 2-5 son paralelizables entre sí. Todas dependen de la tarea 1.

### Task 2: ADR-0001 + ADR-0002

**Descripción:** la escritura asíncrona y el abandono de gRPC. Son la misma cadena causal: al no haber streaming
de bytes, el Extractor recibe el documento completo en un cuerpo multipart, con techo de tamaño.

**Criterios de aceptación:**
- [ ] Las cuatro secciones (`Contexto`, `Decisión`, `Alternativas consideradas`, `Consecuencias`) tienen
      contenido real en ambos ADR.
- [ ] ADR-0001 deja escrito que **nada se ingiere hasta que todo el pipeline pasó**, y por qué el `XADD` va
      después del resumen.
- [ ] ADR-0001 registra el Límite 1 del plan (documento invisible): qué ocurre, en qué ventana, qué ve el
      cliente, y que la cura de fondo es un reconciliador que contradice el rol de solo-productor.
- [ ] ADR-0002 dice explícitamente que renunciar a gRPC **descarta el streaming de bytes**: el Extractor
      recibe el documento completo como cuerpo multipart en RAM, sujeto a un techo de tamaño.
- [ ] ADR-0002 nombra gRPC como alternativa considerada con su costo de reversión (stubs de imagen, un modo de
      falla más, contratos `.proto` que nadie compila todavía).
- [ ] `grep -c "TODO"` en ambos archivos devuelve 0.

**Verificación:**
- [ ] Manual: las alternativas consideradas de ADR-0001 nombran al menos la vía REST de alta (rechazada por
      hub & spoke) y un bus distinto de Redis Streams.

**Dependencias:** 1

**Archivos:** `docs/adr/0001-escritura-asincrona-por-redis-streams.md`, `docs/adr/0002-http-interno-uniforme-sin-grpc.md`

**Alcance:** M

---

### Task 3: ADR-0003 + ADR-0004

**Descripción:** quién calcula el checksum, y qué identidad atraviesa el sistema.

**Criterios de aceptación:**
- [ ] ADR-0003 fija `checksum = sha256(texto extraído)`, calculado por el Orquestador y persistido tal cual.
- [ ] ADR-0003 nombra como alternativas: calcularlo el consumidor (rechazado: dos valores posibles para el
      mismo contenido) y hashear el documento binario (rechazado: el Persistidor nunca ve el binario, y dos
      PDFs distintos con el mismo texto darían checksums distintos).
- [ ] ADR-0003 registra que **cambiar el Extractor invalida los checksums guardados** (Límite 3 del plan).
- [ ] ADR-0004 fija `X-Document-Id` como único nombre de header y deja escrito que no existe un identificador
      de correlación separado.
- [ ] ADR-0004 separa los dos roles: `document_id` identifica **la petición de ingesta** (un reintento del
      cliente genera otro) y el **checksum** es la identidad reintentable (deduplicación).
- [ ] ADR-0004 nombra `X-Correlation-ID` y `X-Request-ID` como alternativas rechazadas, con el motivo.
- [ ] `grep -c "TODO"` en ambos devuelve 0.

**Verificación:**
- [ ] Manual: la separación entre los dos roles sobrevive a la pregunta "¿qué pasa si el cliente reintenta?".

**Dependencias:** 1

**Archivos:** `docs/adr/0003-checksum-calculado-por-el-productor.md`, `docs/adr/0004-identidad-unica-document-id.md`

**Alcance:** M

---

### Task 4: ADR-0006 + ADR-0007

**Descripción:** el estado de resumen y su evento, y la ausencia de autenticación con sus topes por réplica.

**Criterios de aceptación:**
- [ ] ADR-0006 documenta el cambio de nombre del evento, `summary` → `summary_resolved`, como **delta de
      contrato para el Persistidor**, citando `ARCHITECTURE_REPOSITORIES.md` §4 y §9 como los lugares a tocar.
- [ ] ADR-0006 deja escrito que `summary_resolved` existe porque el Orquestador es el único que sabe si el
      cliente pidió resumen, y que el consumidor no debe inferirlo.
- [ ] ADR-0006 deja escrito que con el prefijo `/api/v1` mantenido, el proxy es reenvío transparente sin
      reescritura de rutas.
- [ ] ADR-0007 cumple la nota **OBLIGATORIO** del template: los topes de concurrencia son por réplica y el
      agregado multiplica. Va en consecuencias negativas, destacado.
- [ ] ADR-0007 nombra la autenticación como extensión futura (middleware nuevo en `internal/api`, dominio
      intacto).
- [ ] `grep -c "TODO"` en ambos devuelve 0.

**Verificación:**
- [ ] Manual: `summary` y `summary_resolved` no aparecen ambos como nombre del mismo evento en ningún
      documento de este repo fuera de `ARCHITECTURE_REPOSITORIES.md` y del delta.

**Dependencias:** 1. **Coordinación:** leer la tarea 2 apenas esté, porque ADR-0001 y ADR-0006 describen los
mismos eventos.

**Archivos:** `docs/adr/0006-estado-unico-y-evento-summary-resolved.md`, `docs/adr/0007-servicio-publico-sin-autenticacion.md`

**Alcance:** M

---

### Task 5: ADR-0005 + ADR-0008 (nuevo)

**Descripción:** el borde público, y el contrato de admisión completo. ADR-0008 es nuevo: cubre las 7
decisiones que ningún ADR existente contiene.

**Criterios de aceptación:**
- [ ] ADR-0005 fija que Traefik enruta solo hacia el Orquestador, que el Persistidor se alcanza por URL
      interna y que el Orquestador **nunca** entra a la red de base de datos.
- [ ] ADR-0005 nombra Traefik en modo servidor-detrás de proxy como alternativa, con el punto único de falla
     que introduce.
- [ ] **ADR-0008 (nuevo)** cubre: (a) la semántica del 202 y qué se puede garantizar con él; (b) el 502 cuando
      el Resumidor falla después de una extracción exitosa, con el fallback de reintentar sin resumen;
      (c) la renumeración de las puertas a 1 y 2, con la nota de que el hueco 1/3 se cerró a propósito y de que
      no se inventó lógica nueva; (d) que las alternativas descartadas duplicaban conocimiento de los workers;
      (e) el formato admitido: PDF en el MVP, extensible por decisión posterior;
      (f) la regla de que un semáforo angosto de resumen en el camino de la petición **descarga** carga, y
      por eso el 429 con `Retry-After` es obligatorio y no opcional;
      (g) `POST /api/v1/documents/{document_id}/summary` como caso de uso del Orquestador, con delta cero al
      Persistidor.
- [ ] El nombre del archivo sigue el patrón `0008-<kebab-case>.md`.
- [ ] `grep -rc "TODO" docs/adr/` devuelve 0 en los ocho archivos.

**Verificación:**
- [ ] Manual: las 13 decisiones de la tabla de `tasks/plan.md` tienen una sección que las sostiene.

**Dependencias:** 1

**Archivos:** `docs/adr/0005-traefik-solo-en-el-borde-publico.md`, `docs/adr/0008-contrato-de-admision.md` (nuevo)

**Alcance:** M

---

### Checkpoint: Fase 2 — revision humana

- [ ] Los 8 ADR tienen sus 4 secciones con contenido real
- [ ] Ningún título de ADR contradice su cuerpo
- [ ] Las 13 decisiones tienen un ADR que las sostiene
- [ ] `grep -rc "TODO" docs/adr/` → 0 en los ocho

---

## Fase 3 — Contratos

> Se derivan de los ADR. Secuencial: las tareas 7, 8 y 9 editan el mismo archivo.

### Task 6: `docs/contracts/events.md`

**Descripción:** el contrato del stream, desde la perspectiva del productor. Hoy el repo solo tiene la
perspectiva del consumidor, en `ARCHITECTURE_REPOSITORIES.md` §9.

**Criterios de aceptación:**
- [ ] `original` con payload completo, tabla de campos y tipos. Hoy es `{}`.
- [ ] `summary_resolved` con lo mismo.
- [ ] Cada evento lleva solo **su** pata de tiempo: `original.extraction_time_ms`,
      `summary_resolved.summary_time_ms`. El total no viaja en ningún evento.
- [ ] `schema_version` por mensaje, y el consumidor debe ignorar campos desconocidos.
- [ ] Queda escrito que `original` se publica **después** de que el resumen resolvió, y que
      `summary_resolved` solo existe si `summarize=true`.
- [ ] La sección de reintentos, DLQ y duplicados está escrita desde el productor: qué garantiza, qué no, y qué
      espera el consumidor de este servicio.
- [ ] `grep -c "TODO"` devuelve 0.

**Verificación:**
- [ ] Manual: cada nombre de campo de `events.md` aparece igual en `openapi.yaml` (tarea 7) o queda justificado
      por qué difieren.

**Dependencias:** 2, 3, 4, 5

**Archivos:** `docs/contracts/events.md`

**Alcance:** M

---

### Task 7: `docs/contracts/openapi.yaml` — schemas y vocabulario de `code`

**Descripción:** los tres schemas que hoy son comentarios, más el vocabulario cerrado de códigos de error.

**Criterios de aceptación:**
- [ ] `ExtractAccepted`, `Document` y `Error` con propiedades tipadas, no comentarios.
- [ ] Los valores de los ejes de estado coinciden **exactamente** con los que nombra `CONTEXT.md` (tarea 1). Si
      difieren, se corrige el glosario, no el schema.
- [ ] Vocabulario cerrado de `code`: tipo enumerado, con los valores que las operaciones usan
      (`document_not_found`, `summary_not_available`, `summary_already_resolved`,
      `unsupported_media_type`, `payload_too_large`, `extraction_failed`, `rate_limited`,
      `concurrency_limited`, `summary_failed`, `checksum_conflict`, `invalid_request`, `internal`).
- [ ] `ExtractAccepted` expone `processing_time_ms` como **suma**, más `extraction_time_ms` y
      `summary_time_ms` por separado. Nada se llama `extraction_time_ms` cuando es un total.
- [ ] `Document` expone el `document_id` público y **no** los identificadores internos del Persistidor.
- [ ] El YAML parsea.

**Verificación:**
- [ ] `python3 -c "import yaml; yaml.safe_load(open('docs/contracts/openapi.yaml'))"`
- [ ] Manual: cada valor del enum de estado aparece en `CONTEXT.md`.

**Dependencias:** 1, 6

**Archivos:** `docs/contracts/openapi.yaml`

**Alcance:** M

---

### Task 8: `docs/contracts/openapi.yaml` — las operaciones

**Descripción:** las 8 operaciones con sus parámetros y su matriz de errores. **La operación de ingesta se
escribe y se revisa antes que las otras siete**: es el contrato central y el que más se improvisa.

**Criterios de aceptación:**
- [ ] `POST /api/v1/documents/extract`: multipart con `document` y `summarize`; **202 + `Location`**;
      400 · 413 · 415 · 422 · 429 · 502 · 503.
- [ ] 422 documenta que el motivo viene del enum del Extractor, no de una taxonomía propia.
- [ ] 502 documenta explícitamente "el Resumidor no respondió después de una extracción exitosa, y por esa
      Decisión 3 no se ingirió nada".
- [ ] 429 documenta `Retry-After`, y que los topes son por réplica.
- [ ] Las 7 operaciones de lectura y borrado están completas, con sus parámetros de filtrado y paginación.
- [ ] El 404 de lectura distingue los dos casos: **aún ingerido** (transitorio) y **nunca existió** (la ingesta
      se perdió). Cada uno con su `code`.
- [ ] La regla con plazo está escrita: dentro de la ventana del 202 un 404 es transitorio; pasada la ventana, es
      terminal y el cliente **no** debe reintentar la subida.
- [ ] El YAML parsea.

**Verificación:**
- [ ] `python3 -c "import yaml; yaml.safe_load(open('docs/contracts/openapi.yaml'))"`
- [ ] Manual: la matriz de errores de cada operación enumera solo códigos del vocabulario de la tarea 7.

**Dependencias:** 7

**Archivos:** `docs/contracts/openapi.yaml`

**Alcance:** M

---

### Task 9: `POST /api/v1/documents/{document_id}/summary`

**Descripción:** el endpoint de resumen a demanda. Propiedad del **Orquestador**, no del Persistidor: el
Persistidor no puede llamar al Resumidor sin romper la regla hub & spoke.

**Criterios de aceptación:**
- [ ] La operación está en `openapi.yaml` con 202 + `Location`, 404, 409, 502.
- [ ] El flujo queda escrito: lee el documento por proxy → si ya tiene resumen, 409 → descarga el texto
      extraído por `download/original` → llama al Resumidor → `XADD summary_resolved` → 202.
- [ ] Queda escrito que **es un caso de uso del Orquestador, no una entrada de la allowlist del proxy**. El
      allowlist de `proxy.go` no crece.
- [ ] Queda escrito que el delta al Persistidor es **cero**: usa `GET /api/v1/documents/{document_id}` y
      `GET /api/v1/documents/{document_id}/download/original`, los dos ya en `ARCHITECTURE_REPOSITORIES.md` §10.
- [ ] Las dos consecuencias quedan registradas: consume el mismo semáforo angosto y el mismo timeout holgado
      que la ingesta, y por eso también puede devolver 429; y el Persistidor descarta en silencio un
      `summary_resolved` sobre un documento ya completo (`ARCHITECTURE_REPOSITORIES.md` §4), lo que hace del
      409 previo una guarda de defensa, no la única.
- [ ] El valor propio del endpoint queda escrito: obtener el resumen de un documento ya ingerido sin obligar
      al cliente a re-subir y re-pagar la extracción.

**Verificación:**
- [ ] Manual: el endpoint no requiere ningún cambio en `ARCHITECTURE_REPOSITORIES.md`.

**Dependencias:** 8

**Archivos:** `docs/contracts/openapi.yaml`

**Alcance:** S

---

### Task 10: `docs/contracts/workers.md`

**Descripción:** los contratos del Extractor y del Resumidor. **Es una propuesta**: los dos servicios no
tienen repositorio, así que no hay contraparte.

**Criterios de aceptación:**
- [ ] El encabezado dice que es una **propuesta**, con fecha y con quién debe confirmar.
- [ ] Contrato del Extractor: request, response y el **enum de motivos de extracción**. Este enum es el que
      consumirá la puerta 2, así que se escribe una sola vez.
- [ ] Contrato del Resumidor: request, response y el motivo `document_too_long`.
- [ ] La política de 2 reintentos en error de red, y por qué no se reintenta un error de validación.
- [ ] Una sección que liste los puntos que dependen de la implementación real de cada worker.
- [ ] `grep -c "TODO"` devuelve 0.

**Verificación:**
- [ ] Manual: el enum de motivos coincide con el que la puerta 2 va a consumir, y existe en un solo lugar.

**Dependencias:** 5

**Archivos:** `docs/contracts/workers.md`

**Alcance:** M

---

### Checkpoint: Fase 3 — revision humana

- [ ] El YAML parsea
- [ ] Ningún campo de evento aparece con dos nombres en el repo
- [ ] El enum de motivos está escrito una sola vez
- [ ] Los valores de los ejes de estado coinciden con `CONTEXT.md`

---

## Fase 4 — Documentos derivados

### Task 11: `ARCHITECTURE_ORCHESTRATOR.md`

**Descripción:** el documento más leído del repo y el más desactualizado. Hoy está entero dentro de un bloque
de código y tiene 16 errores técnicos identificados.

**Criterios de aceptación:**
- [ ] Se saca el bloque ```` ```markdown ```` de la línea 1 y el ```` ``` ```` huérfano del final. El
      documento tiene que renderizar como markdown.
- [ ] Los 16 puntos están corregidos:
      L27 gRPC/Protobuf → HTTP interno · L29 `X-Correlation-ID` → `X-Document-Id` · L29 OpenTelemetry fuera ·
      L35 ruta → `/api/v1/documents/extract` · L39 formato admitido · L44 200 → 202 · L47 cuerpo →
      `ExtractAccepted` · L73 `grpc_extractor/` → `http_extractor/` · L77 `proto/` eliminada ·
      L96 `GenerateSummary` → evento `summary_resolved` · L101 `PublishSummaryEvent` →
      `PublishSummaryResolvedEvent` · L112 ruta del diagrama · L130 "Responde 200 OK" → 202 + `Location` ·
      L153 `internal-net` → `mired` · L163 `EXTRACTOR_GRPC_ADDR` → `EXTRACTOR_URL` ·
      L173 `internal-net` → `mired`
- [ ] `cmd/main.go` → `cmd/server/main.go`.
- [ ] Se agregan las piezas que hoy solo existen en los stubs: el pipeline de 8 pasos de `extract.go`, las dos
      puertas de `gates.go`, y los ports con los nombres que definieron los stubs (`Extractor`, `Extraction`,
      `IncomingDocument`), no los antiguos (`ExtractorClient`, `ExtractionResult`, `FileMetadata`).
- [ ] Se registra la línea `POST /api/v1/documents/{document_id}/summary` en la estructura.
- [ ] Cero reescritura de prosa que no sea falsa.

**Verificación:**
- [ ] `grep -rniE "grpc|proto|internal-net|X-Correlation-ID" ARCHITECTURE_ORCHESTRATOR.md` → 0
- [ ] Manual: el documento renderiza; el diagrama de §6 termina en 202 y no en 200.

**Dependencias:** 2-10

**Archivos:** `ARCHITECTURE_ORCHESTRATOR.md`, `internal/application/extract.go` (comentario),
`internal/application/gates.go` (comentario)

**Alcance:** L

---

### Task 12: `Especificaciones_v3.md`

**Descripción:** el documento de requisitos original. **Se preserva**: no se reescribe. Solo se le agregan 5
líneas que lo declaran superado en los puntos donde contradice los contratos cerrados.

**Criterios de aceptación:**
- [ ] Banner en la cabecera que declara el documento superado y nombra las secciones una por una.
- [ ] Marcador inline en §5: la escritura es asíncrona por stream, no HTTP al Persistidor.
- [ ] Marcador inline en §6.A: el campo es `document`, y el conjunto de formatos se define en ADR-0008.
- [ ] Marcador inline en §6.B: el identificador es `document_id`, y el modelo vigente es
      `ARCHITECTURE_REPOSITORIES.md` §8.
- [ ] Marcador inline en §7: las redes se llaman `mired`, no `internal-net`.
- [ ] **5 líneas agregadas, 0 modificadas.** §2, §3, §4 y §8 quedan intactos.
- [ ] `internal-net` sobrevive en §7 legítimamente, y por eso el grep de la capa 1 no barre este archivo.

**Verificación:**
- [ ] `git diff --stat` sobre el archivo: solo adiciones.
- [ ] Manual: alguien que lea solo los marcadores sabe que el documento no es normativo.

**Dependencias:** 2-8

**Archivos:** `Especificaciones_v3.md`

**Alcance:** XS

---

### Checkpoint: Fase 4 — revision humana

- [ ] Cero tokens staleness en `docs/` y `ARCHITECTURE_ORCHESTRATOR.md`
- [ ] `grep -rn "puerta 3" --include='*.go' .` → 0
- [ ] `Especificaciones_v3.md` solo tuvo adiciones

---

## Fase 5 — Cierre

### Task 13: `docs/contracts/delta-persister.md`

**Descripción:** el entregable que revisa el equipo del Persistidor. Sin este documento, los dos repos
divergen y el test de integración falla.

**Criterios de aceptación:**
- [ ] Una fila por cambio, con: qué cambia, el tipo (**rompe** / **aditivo** / **cosmético**), el ADR o contrato
      que lo origina, y el lugar exacto a tocar en `ARCHITECTURE_REPOSITORIES.md`.
- [ ] Destacado que hay **una sola ruptura**: `summary` → `summary_resolved`. Todo lo demás es agregar campos o
      renombrar nada.
- [ ] Las filas cubren: el nombre del evento, `extraction_time_ms` y `summary_time_ms` como campos nuevos con
      `processing_time_ms` como suma, `schema_version` por mensaje, `X-Request-ID` → `X-Document-Id`,
      `internal-net` → `mired`, y el prefijo `/api/v1` en las rutas de §10.
- [ ] Registra que `POST /api/v1/documents/{document_id}/summary` **no requiere ningún cambio** en su lado.
- [ ] Registra que `ARCHITECTURE_REPOSITORIES.md` §10 y §13 se contradicen sobre el prefijo, y cuál se corrige.
- [ ] La condición bloqueante queda escrita: la implementación no arranca hasta el reconocimiento.

**Verificación:**
- [ ] Manual: cada fila cita el ADR o el contrato que la origina.

**Dependencias:** 6, 9

**Archivos:** `docs/contracts/delta-persister.md` (nuevo)

**Alcance:** S

---

### Task 14: barrido de consistencia

**Criterios de aceptación:**
- [ ] `grep -rn "TODO" --include='*.md' --include='*.yaml' CONTEXT.md docs ARCHITECTURE_ORCHESTRATOR.md Especificaciones_v3.md` → 0
- [ ] `grep -rn "puerta 3" --include='*.go' .` → 0
- [ ] `python3 -c "import yaml; yaml.safe_load(open('docs/contracts/openapi.yaml'))"` no falla
- [ ] `git diff --stat Especificaciones_v3.md` muestra solo adiciones
- [ ] Los aciertos del grep de tokens staleness están **revisados uno por uno**, no contados. Cada
      `grpc`/`proto`/`correlation_id`/`X-Correlation-ID`/`internal-net` que sobrevive tiene que estar en una
      frase que lo rechace: los de `0002` (título, contexto, decisión y alternativa) y los de `0004` (nombres
      rechazados). Si alguno afirma el token en vez de rechazarlo, se corrige. Este grep **no** se espera en
      cero, y por eso no se automiza.

**Verificación:** son los comandos de aceptación. Todo en verde.

**Dependencias:** 11, 12, 13

**Archivos:** 0-2, solo correcciones residuales

**Alcance:** XS

---

### Task 15: lectura en frío

**Descripción:** la capa 2 de la verificación. Es la única que mide el objetivo real, porque la capa 1 es ciega
a lo que se escribe nuevo. **Instrucción clave:** el lector debe responder "no está especificado" en vez de
inferir. Sin esa instrucción la prueba no vale nada, porque un agente completa los huecos con sentido común.

**Criterios de aceptación:**
- [ ] Un lector sin contexto responde estas 15 preguntas, cada una con la cita del documento que la sostiene:
      1. ¿Qué status y qué header devuelve la ingesta, y qué header trae el `document_id`?
      2. ¿Qué pasa si el Resumidor falla después de una extracción exitosa? ¿Se ingiere algo?
      3. ¿Se publica algo en el stream antes de que termine el resumen?
      4. ¿Cómo se llama el evento de resumen?
      5. ¿Qué campos de tiempo lleva cada evento, y dónde está el total?
      6. ¿Qué significa el checksum y quién lo calcula?
      7. Si el cliente reintenta la misma subida, ¿obtiene el mismo `document_id`? ¿Cómo se deduplica?
      8. ¿Cuántas puertas hay, qué revisa cada una, y en qué orden corren?
      9. ¿Qué formatos se aceptan y dónde está el techo de tamaño?
      10. ¿El proxy reescribe las rutas hacia el Persistidor?
      11. ¿Qué es un documento abandonado, quién lo detecta y qué pasa con su resumen?
      12. ¿Qué devuelve un 404 de lectura y cuándo es transitorio?
      13. ¿Quién puede pedir el resumen de un documento ya ingerido, y qué le cuesta al Persistidor?
      14. ¿Los topes de concurrencia son globales o por réplica?
      15. ¿Qué le falta a este contrato que un implementador podría no adivinar?
- [ ] La pregunta 15 tiene una respuesta honesta, y si dice "no está especificado" se acepta como **hallazgo
      abierto**, no como falla: significa que el plan quedó incompleto y se agrega a las preguntas abiertas
      de `tasks/plan.md`.
- [ ] Cada respuesta incorrecta o "no está especificado" se corrige en el documento que debería sostenerla, y se
      vuelve a preguntar.

**Verificación:**
- [ ] Manual: leer `tasks/plan.md` §Límites conocidos y comparar. La lista de 15 preguntas y la de límites
     deben cubrir lo mismo.

**Dependencias:** 14

**Archivos:** los que la lectura en frío señale

**Alcance:** S, más las correcciones que surjan

---

### Checkpoint: Fase 5 — revision humana final

- [ ] Los 2 documentos nuevos revisados
- [ ] El delta cross-repo entregado al equipo del Persistidor
- [ ] La lectura en frío sin huecos, o con los huecos registrados como preguntas abiertas