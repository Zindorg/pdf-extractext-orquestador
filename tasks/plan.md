# Plan: cerrar los contratos y los ADR del Orquestador

> Documento de plan. La lista de tareas con criterios de aceptación está en `tasks/todo.md`.

## Alcance

Solo documentación. **Cero lógica de código.** El alcance permite tocar comentarios dentro de archivos `.go`
cuando un comentario contradice una decisión ya tomada (caso de la renumeración de las puertas), pero no
agregar, modificar ni eliminar ninguna declaración.

Alcance de este plan:

- **Sí:** `CONTEXT.md`, `docs/adr/*.md`, `docs/contracts/*`, `ARCHITECTURE_ORCHESTRATOR.md`,
  `Especificaciones_v3.md`, y dos archivos nuevos (`docs/adr/0008-*.md`, `docs/contracts/delta-persister.md`).
- **No:** `ARCHITECTURE_REPOSITORIES.md`. Es el blueprint del Persistidor, que vive en este repo pero pertenece
  a otro equipo. Las correcciones que necesite se detallan en `docs/contracts/delta-persister.md`, no se aplican.
- **No:** código, `go.mod`, `Makefile`, `Dockerfile`, `docker-compose.yml`.

### Por qué este orden

El repo hoy tiene 72 marcas `TODO` en sus documentos y **cero implementación**: los 27 archivos `.go` tienen
3 líneas cada uno (sentencia `package` más un comentario). El contenido real está en los documentos, y varios
de ellos se contradicen entre sí. El objetivo de esta fase es que la spec de implementación se pueda escribir
después sin tomar ninguna decisión, y que ningún documento contradiga a otro.

## Decisiones cerradas

| # | Decisión | Dónde vive |
|---|---|---|
| 1 | `POST /api/v1/documents/extract` responde **202 + `Location`**, no 200 con el texto | ADR-0008, `openapi.yaml` |
| 2 | Extracción **y** resumen ocurren antes de responder. Nada se ingiere hasta que todo salió bien | ADR-0001, ADR-0008 |
| 3 | Si el Resumidor falla después de una extracción exitosa: **502, nada se ingiere** | ADR-0008 |
| 4 | El evento de resumen se llama **`summary_resolved`** | ADR-0006, `events.md` |
| 5 | `processing_time_ms` es la **suma** de las patas; cada evento lleva solo la suya | ADR-0008, `events.md` |
| 6 | Identidad única `document_id` + header `X-Document-Id`. `document_id` identifica la petición de ingesta; el **checksum** es la identidad reintentable | ADR-0004 |
| 7 | Comunicación interna **HTTP en los tres servicios**. Sin gRPC, sin streaming de bytes | ADR-0002 |
| 8 | El Persistidor mantiene el prefijo `/api/v1` → el proxy es reenvío transparente, sin reescritura | ADR-0006, `openapi.yaml` |
| 9 | Dos puertas, numeradas en orden de ejecución: 1 (borde) y 2 (clasificación por motivo) | ADR-0008 |
| 10 | Sin autenticación en el MVP. Los topes de concurrencia son **por réplica** | ADR-0007 |
| 11 | `POST /api/v1/documents/{document_id}/summary`: propiedad del **Orquestador**. Delta cero al Persistidor | `openapi.yaml`, ADR-0008 |
| 12 | El formato admitido en el MVP es **PDF**, extensible por decisión posterior | ADR-0008 |
| 13 | El hueco del documento invisible se **documenta**, no se construye. El contrato incluye una regla con plazo para el 404 | ADR-0001, `openapi.yaml` |
| 14 | Framework HTTP: **Gin** (comentario de implementación, no ADR). No asume tareas nativas de Traefik: borde, TLS y balanceo quedan en el edge (ADR-0005) | `ARCHITECTURE_ORCHESTRATOR.md` §2 |

## Pipeline

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

**Por qué el guardado va en el paso 7 y no antes.** Es la única posición donde "202 = exitoso" es honesto.
Guardar antes de resumir significa que un fallo del Resumidor deja el documento ingestado y el cliente sin
`document_id`: texto en la base ocupando su checksum, invisible, y re-subidas posteriores descartadas como
duplicado. Guardando al final, o se guardó todo y se avisó éxito, o no se guardó nada.

**El costo de esa elección:** una caída del Resumidor devuelve 502 en las peticiones con `summarize=true` y
descarta una extracción que sí funcionó. El radio de impacto queda limitado a los clientes que piden resumen,
y el fallback es reintentar con `summarize=false`, que funciona completo. Un resumen perdido es recuperable;
un documento invisible con su checksum ocupado no.

## Estructura del trabajo

Cinco fases. La lista completa de tareas está en `tasks/todo.md`.

| Fase | Tareas | Entregable |
|---|---|---|
| 1 | 1 | Vocabulario del dominio |
| 2 | 2-5 | 8 ADR con contenido real |
| 3 | 6-10 | Contratos cerrados |
| 4 | 11-12 | Documentos derivados alineados |
| 5 | 13-15 | Delta cross-repo y verificación |

## Dependencias y paralelización

```
        T1 (glosario)
             │
    ┌────────┼────────┬────────┐
   T2       T3       T4       T5        ← paralelizables entre sí
  0001      0003     0006    0005+0008
   0002      0004     0007
    │        │        │        │
    └────────┴────┬───┴────────┘
                  │
    ┌─────────────┼─────────────┐
   T6           T7            T10
  events.md   openapi       workers.md
              schemas
                  │
                 T8  (operaciones, ingest primero)
                  │
                 T9  (endpoint de resumen)
                  │
        ┌─────────┴─────────┐
       T11                T12
  ARCHITECTURE      Especificaciones
  _ORCHESTRATOR        _v3.md
        └─────────┬─────────┘
                  │
        ┌─────────┼─────────┐
       T13       T14       T15
     delta    barrido   lectura
                            en frío
```

**Paralelizable:** las tareas 2-5 (ADR), porque son documentos independientes y todas dependen solo del
glosario.

**Riesgo de paralelización:** ADR-0001 (tarea 2) y ADR-0006 (tarea 4) describen los mismos eventos. Si se
escriben en paralelo pueden divergir en nombres de campo. Quien tome la tarea 4 debe leer la 2 apenas esté.

**Secuencial obligatorio:** todo lo demás. En particular las tareas 7, 8 y 9 editan el mismo archivo y no
pueden solaparse.

## Verificación en dos capas

**Capa 1 — mecánica.** Solo sobre los archivos que nos pertenecen. `ARCHITECTURE_REPOSITORIES.md` queda
excluido porque pertenece al Persistidor.

```bash
grep -rn "TODO" --include='*.md' --include='*.yaml' \
  CONTEXT.md docs ARCHITECTURE_ORCHESTRATOR.md Especificaciones_v3.md          # → 0
grep -rn "puerta 3" --include='*.go' .                                        # → 0
python3 -c "import yaml; yaml.safe_load(open('docs/contracts/openapi.yaml'))"
```

Y un grep que **no** se espera en cero, sino que se revisa a mano:

```bash
grep -rniE "grpc|proto|correlation_id|X-Correlation-ID|internal-net" \
  --include='*.md' --include='*.yaml' docs ARCHITECTURE_ORCHESTRATOR.md
```

Este tiene dos categorías de aciertos legítimos, y por eso no puede ser un `→ 0`:

- **`docs/adr/0002-http-interno-uniforme-sin-grpc.md`** menciona `gRPC` y `proto/` en el título, en el contexto
  (lo que proponía el blueprint viejo), en la decisión ("sin gRPC") y en la alternativa rechazada. Es
  exactamente donde las menciones tienen que estar. `ADR-0004` menciona `X-Correlation-ID` y `X-Request-ID` por
  la misma razón: los dos nombres rechazados.
- **`Especificaciones_v3.md` §7** conserva `internal-net` porque el documento se preserva (tarea 12). §7 está
  fuera del alcance del grep de arriba.

El criterio de fondo no es "la palabra no aparece", sino **"ningún documento afirma gRPC como el transporte"**.
Se verifica leyendo los aciertos: cada uno tiene que estar en una frase que lo rechace.

**Capa 2 — lectura en frío.** Es la que mide el objetivo real, porque la capa 1 es ciega a lo que se escribe
nuevo. Al final, un agente sin contexto recibe los documentos y responde las 15 preguntas de la sección
"Límites conocidos". Si responde "no está especificado" donde el plan dice que debería haber respuesta, la
fase no está cerrada.

## Tests de la lógica profunda (para la fase de implementación)

El análisis TDD del proyecto anterior (`analisis-tdd.md`) confirmó el patrón más caro de ese repo: **el módulo
con más lógica de negocio (`PDFExtraction`) no tenía ningún test**, y por eso los refactors se aplicaban sin
que la suite los detectara. Acá la lógica equivalente son las puertas 1-2, el checksum y el orden de las dos
`XADD`. Reglas para cuando se escriba la suite:

- La **lógica de aplicación** (pipeline) se testea a través del seam del caso de uso (`ExtractUseCase`), con
  **fakes en memoria** de los puertos (`ExtractorFake`, `SummarizerFake`, `EventPublisherSpy`) — no con mocks
  que devuelven el valor esperado (anti-patrón A1 del análisis previo).
- Las **puertas** se testean como funciones puras del dominio: gate 1 (tamaño/tipo/cifrado → `413`/`415`/`400`)
  y gate 2 (motivo del enum → `422`; motivo fuera del enum → `503`). Tabla de pares entrada → salida.
- El **orden de publicación** (nada antes de que termine el resumen; `original` antes de `summary_resolved`) se
  asevera con un spy de `EventPublisher`, no revisando Redis.
- Los **adaptadores** (`http_extractor`, `http_summarizer`) se testean como contract tests contra un servidor
  real (ADR-0002); el contrato no se mockea.
- El **checksum** se testea contra una fuente de verdad independiente (64 hex minúsculas), no recomputando la
  misma función del código (patrón del análisis previo).
- Evitar tests sin aserción, de forma (`inspect`/`hasattr` equivalentes en Go) y verificación por canales
  internos (anti-patrones A2/A3/A4 del análisis previo).

## Lecciones del proyecto anterior

Los tres análisis de PDF-Extractext (`analisis-arquitectura.md`, `analisis-clean-code.md`,
`analisis-tdd.md`) se revisaron contra lo documentado en este repo. No son el mismo proyecto: se comparan los
patrones, no las líneas. Qué se adoptó (o ya se evitaba), y qué queda diferido a la implementación:

| Corrección previa | Estado en este repo |
|---|---|
| Doble capa de paso (`use_cases` + `services`) | Evitado por diseño: una sola capa de aplicación con pipeline profundo (ARCHITECTURE §4) |
| `PDFUpload` y `sanitize_filename` como código muerto | No aplica: cero código en esta fase; la decisión "no construir el reconciliador" se documentó (Límite 1) en vez de dejarse implícita |
| Seam para el extractor de PDF | Resuelto: puerto `Extractor` definido por el consumidor (ADR-0002, ARCHITECTURE §5) |
| Modelo de dominio anémico | Diferido: los invariantes quedaron registrados en ADR-0003 (checksum) y ADR-0006 (estados), pero el valor vivo en `types.go` se escribe en la implementación |
| Documentación de arquitectura sincronizada | Resuelto en esta fase: tareas 11-14 alinearon `ARCHITECTURE_ORCHESTRATOR.md` y marcaron `Especificaciones_v3.md` |
| Respuesta/DTO duplicado 3× y mapeo manual | Resuelto a nivel contrato: schemas únicos en `openapi.yaml`, sin mapeo manual (DTOs en el adaptador) |
| Flujo validar+checksum+dedup duplicado | Evitado: un solo pipeline y las decisiones de dedup viven en ADR-0003 |
| Boilerplate `ObjectId` ×5 | No aplica (el Orquestador no toca Mongo); vive en el repo del Persistidor |
| Timestamps con doble origen | Resuelto y mejor: cada evento lleva solo su pata de tiempo, el total se calcula (ADR-0008, `events.md`) |
| Dependencia inyectada sin uso | Evitado por la regla "el puerto lo define su consumidor"; el port de Persister separado por rol (ARCHITECTURE §5) refuerza que no se inyecte más que lo usado |
| Interfaz de 8 métodos (ISP débil) | Resuelto: puertos de 1 método; `Persister` descompuesto en `PersisterReader` + `PersisterDeleter` siempre que se use (ARCHITECTURE §5) |
| Módulo con más lógica sin tests (`PDFExtraction`) | Diferido: reglas escritas arriba en "Tests de la lógica profunda" |
| No existía `CONTEXT.md` para los seams | Resuelto: `CONTEXT.md` + `CONTEXT-MAP.md` + puertos declarados en ARCHITECTURE §5 |
| Tests tautológicos, sin aserción, por canales internos | Diferido: prohibidos explícitamente en "Tests de la lógica profunda" |

## Riesgos

| Riesgo | Impacto | Mitigación |
|---|---|---|
| `workers.md` describe contratos de dos servicios sin repositorio | Alto | Tarea 10 lo deja marcado como propuesta con fecha, y una condición bloqueante exige recontrastarlo antes de implementar el adaptador del Extractor |
| El delta al Persistidor no es reconocido por el otro equipo | Alto | Condición bloqueante: la implementación no arranca hasta el reconocimiento |
| Los documentos quedan más consistentes a mitad de camino que al final | Medio | Tarea 1 agrega un banner temprano en `ARCHITECTURE_ORCHESTRATOR.md`, el documento más leído del repo |
| `workers.md` da una sensación de cierre que no corresponde | Medio | Marcado como v0 sin contraparte; el riesgo se registra en lugar de ocultarse |

## Límites conocidos

Se registran en lugar de construirse. Cada uno dice qué lo resolvería.

1. **Documento invisible.** Si el proceso muere entre los dos `XADD`, el documento queda ingestado y el
   cliente nunca recibe el `document_id`. Al re-subir, el Persistidor descarta por checksum duplicado y el
   cliente recibe un 202 de un `document_id` que no existe. La ventana es de milisegundos y cualquier flujo
   publicar-luego-acknowledger la tiene.
   *Mitigación parcial:* el contrato incluye una regla con plazo para el 404, que corta el bucle de
   reintentos del cliente.
   *Curación de fondo:* un reconciliador que re-lance los documentos sin resumen. Contradice que el Orquestador
   sea solo productor del stream, así que es un componente nuevo cross-repo.

2. **`workers.md` es un acuerdo con nosotros mismos.** El enum de motivos que se escriba ahí es el que
   consumirá la puerta 2. Si el repositorio del Extractor tiene otros modos de fallo, el contrato se mueve.

3. **Cambiar el Extractor invalida los checksums guardados.** El checksum es `sha256` del texto extraído. Cualquier
   cambio en el extractor cambia el texto, cambia el checksum, y re-ingerir un PDF "idéntico" se vuelve un
   documento nuevo en vez de un duplicado. Registrar en ADR-0003.

## Preguntas abiertas

Ninguna bloquea esta fase. Todas bloquean fases posteriores.

1. ¿El equipo del Extractor y del Resumidor confirma `docs/contracts/workers.md`?
2. ¿El equipo del Persistidor reconoce `docs/contracts/delta-persister.md`?
3. ¿El hueco del documento invisible (Límite 1) aparece en producción con frecuencia real? Si aparece, se
   ataca con los datos a la vista: idempotencia por clave del cliente, o el reconciliador.
4. ¿Qué algoritmo de reintento usa el cliente ante un `404` transitorio? El contrato solo dice "espera
   creciente" y fija la ventana en 60 segundos; los valores concretos (nº de intentos, backoff) son política
   del cliente y nadie la definió todavía.
5. ~~¿Qué tope de tamaño tienen `extracted_text` y `summary` como campos del evento?~~ **Cerrada: no hay techo
   nuevo.** Existe el techo de 10 MiB del archivo en la puerta 1. El texto extraído de un PDF no puede superar
   el contenido del propio archivo (no hay OCR en el MVP, y fuentes/imágenes/estructura ocupan espacio que no
   es texto), así que queda acotado por el mismo techo del archivo sin límite adicional. Redis Streams acepta
   mensajes hasta `proto-max-bulk-len` (512 MiB por defecto), muy por encima de ese tope. Si en el futuro
   aparecen formatos que expandan el texto más allá del archivo, se define un límite explícito entonces;
   hasta hoy no hay caso que lo motive (verbatim en `docs/contracts/events.md` §"Sobre el tamaño de los campos
   del evento").