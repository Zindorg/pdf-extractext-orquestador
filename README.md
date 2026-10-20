# pdf-extractext-orquestador

Punto de entrada público del sistema **PDF-Extractext**. Es el **Orquestador** de la coordinación del procesamiento de documentos: recibe la solicitud del cliente, encadena extracción, resumen opcional e ingesta, publica el resultado en Redis Streams y actúa como **único punto de contacto** con el resto de los servicios.

- **Zero-Disk**: ningún servicio escribe el documento en disco; todo existe en RAM mientras dura la solicitud.
- **Asíncrono por construcción**: la respuesta al cliente no espera a que el documento sea consultable.
- **Dos ejes de estado independientes**: `delivery_status` (INGESTED/AVAILABLE) y `summary_status` (NOT_REQUESTED/PENDING/RESOLVED/FAILED).
- **Duplicados silenciosos**: detectados por `checksum` (sha256 del texto extraído), no son errores.
- **Doble Traefik**: `traefik-public` en el borde (80/443, TLS, límites) y `traefik-internal` en `mired` (ruteo a workers por `*.internal`).

> Ver **[CONTEXT.md](CONTEXT.md)** para el glosario completo de términos (Documento, Identificador de documento, Texto extraído, Resumen, Checksum, Duplicado, Estado de entrega, Estado de resumen, Ingesta, Documento abandonado, Zero-Disk).

---

## 1. Requisitos

- **Docker** y **Docker Compose**
- **Go 1.27.1** (para compilar localmente, test y mocks)
- **Python/uv no se usan**: es un servicio Go.

---

## 2. Variables de Entorno

Cada despliegue necesita un `.env`. Copiar `.env.example` como base:

```bash
cp .env.example .env
```

| Variable | Valor por defecto | Descripción |
|---|---|---|
| `VERSION` | `0.1.0` | Tag de imagen Docker (`pdf-extractext-orquestador:v${VERSION}`) |
| `TZ` | `America/Argentina/Mendoza` | Zona horaria |
| `PUBLIC_HOST` | `api.example.com` | Host público usado por Traefik (router `Host`) |
| `PUBLIC_BASE_URL` | `https://api.example.com` | Base URL pública para generar `Location` absoluto en respuestas |
| `PUBLIC_TLS` | `false` | `true` para habilitar TLS en router Traefik |
| `RATE_LIMIT_AVERAGE` | `100` | Rate limit promedio (requests/s) en borde |
| `RATE_LIMIT_BURST` | `200` | Rate limit burst en borde |
| `STREAM_NAME` | `document-events` | Stream Redis para eventos de ingesta |
| `MAX_DOCUMENT_BYTES` | `10485760` | Tamaño máximo del documento (10 MiB). Aplicado en borde y a nivel de configuración |
| `EXTRACTOR_TIMEOUT` | `30s` | Timeout HTTP hacia el Extractor |
| `SUMMARIZER_TIMEOUT` | `120s` | Timeout HTTP hacia el Resumidor (holgado, modelo local serializado) |
| `EXTRACT_MAX_IN_FLIGHT` | `16` | Semáforo de concurrencia para `POST /api/v1/documents/extract` |
| `SUMMARY_MAX_IN_FLIGHT` | `4` | Semáforo de concurrencia para `POST /api/v1/documents/{id}/summary` |
| `LOG_LEVEL` | `info` | Nivel de log (`debug/info/warn/error`) |
| `DOCKER_API_VERSION` | `1.44` | Versión de API Docker para Traefik (robustez frente a daemons nuevos) |
| `EXTRACTOR_MOCK_DELAY_MS` | `0` | (Solo mocks) Demora artificial en Extractor mock (`lento.pdf`) |
| `SUMMARIZER_MOCK_DELAY_MS` | `0` | (Solo mocks) Demora artificial en Resumidor mock |

> Las URLs internas hacia workers (`extractor.internal`, `summarizer.internal`, `persister.internal`) y la dirección de Redis (`redis:6379`) están fijas en los composes (topología `mired`).

---

## 3. Bootstrap de Redes y Volúmenes

Se necesitan dos redes Docker externas:

```bash
docker network create public-net
docker network create mired
```

Traefik-public crea el volumen `traefik-public-acme` para certificados ACME (en `compose.traefik.yml`). Redis crea el volumen `redis-data`.

---

## 4. Despliegue con Docker Compose (modo producción)

El `compose.yml` es paraguas e incluye: `compose.orchestrator.yml`, `compose.redis.yml`, `compose.traefik.yml`.

```bash
cp .env.example .env  # ajustar valores
docker network create public-net mired || true
docker compose up --build -d
```

Qué se levanta:

- **traefik-public** (v3.6): borde en `80:80`/`443:443`, red `public-net`. Enruta `Host(${PUBLIC_HOST}) && PathPrefix(/api/v1)`.
- **traefik-internal** (v3.6): red `mired`, expone aliases `extractor.internal`, `summarizer.internal`, `persister.internal`. Provider docker con `exposedByDefault=false`.
- **redis**: Redis 7 con AOF (`--appendonly yes`), solo en `mired`. Stream `document-events`.
- **orchestrator**: Go multi-stage (distroless, nonroot), en `public-net` + `mired`, **sin puertos al host**. Healthcheck reusa el binario (`/app/server healthcheck`).

Comprobar estado:

```bash
docker compose ps
docker compose logs -f orchestrator
curl -s https://api.example.com/api/v1/health  # o vía host local
```

---

## 5. Smoke Local con Mocks (#20)

Para levantar el flujo end-to-end **sin los repos externos** (Extractor/Resumidor/Persistidor reales), usar el override `compose.mocks.yml`. Los mocks se registran en `traefik-internal` con las mismas URLs internas.

```bash
docker network create public-net mired || true
docker compose -f compose.yml -f compose.mocks.yml up --build -d
```

El orquestador queda expuesto internamente en `http://orchestrator:8080` (mired). Probar extract con resumen:

```bash
cat > /tmp/contrato.pdf << 'PDF'
%PDF-1.7
PDF

docker run --rm --network mired -v /tmp/contrato.pdf:/tmp/contrato.pdf:ro \
  curlimages/curl:latest -s -X POST http://orchestrator:8080/api/v1/documents/extract \
  -F "document=@/tmp/contrato.pdf" -F "summarize=true"
```

Respuesta esperada (`202`): `document_id`, `delivery_status=INGESTED`, `summary_status=RESOLVED`, `summary`, `processing_time_ms`, `extraction_time_ms`, `summary_time_ms`.

Casos útiles de prueba (controlados por nombre de archivo en el Extractor mock):

| Archivo | Comportamiento |
|---|---|
| `contrato.pdf` | 200 success |
| `vacio.pdf` | Texto extraído vacío → puerta 2 rechaza (no se publica) |
| `cifrado.pdf` | 422 `encrypted_pdf` |
| `roto.pdf` | 422 `corrupt` |
| `escaneado.pdf` | 422 `unsupported_features` |
| `lento.pdf` | Demora (`EXTRACTOR_MOCK_DELAY_MS`) |

Resumidor mock: texto que contenga `[demasiado-largo]` → 422 `document_too_long` (mapeado a 502 en API pública). Texto con `[resumen-vacio]` → summary vacío (rechazado por caso de uso antes de publicar).

---

## 6. Endpoints (resumen)

La API pública vive bajo `/api/v1`. Ver **[docs/contracts/openapi.yaml](docs/contracts/openapi.yaml)** para el contrato completo.

| Método | Ruta | Descripción |
|---|---|---|
| `POST` | `/api/v1/documents/extract` | Ingresa documento (multipart: `document`, `summarize` bool). Responde `202 Accepted` con `Location`. |
| `POST` | `/api/v1/documents/{document_id}/summary` | Solicita resumen a demanda (ya existe el documento). `409` si `RESOLVED` o `PENDING`. |
| `GET` | `/api/v1/documents/{document_id}` | Consulta documento (proxy al Persistidor) |
| `GET` | `/api/v1/documents` | Listado (proxy) |
| `GET` | `/api/v1/documents/{document_id}/download/original` | Descarga texto extraído (texto plano) |
| `GET` | `/api/v1/documents/{document_id}/download/summary` | Descarga resumen (409 si `PENDING`) |
| `DELETE` | `/api/v1/documents/{document_id}` | Soft-delete (proxy) |
| `POST` | `/api/v1/documents/{document_id}/restore` | Restaura (proxy) |
| `GET` | `/health` | Liveness |
| `GET` | `/ready` | Readiness (Redis + disponibilidad general) |

Mapeo dominio→HTTP (en `internal/api/errors.go`): `invalid_request`→400, `document_not_found`/`summary_not_available`→404, `summary_pending`/`summary_already_resolved`/`checksum_conflict`→409, `extraction_failed`→422, `payload_too_large`→413, `unsupported_media_type`→415, `extractor_unavailable`/`summary_failed`→5xx según caso, `concurrency_limited`→429 (+`Retry-After`).

---

## 7. Arquitectura y Contratos

- **Casos de uso**: `internal/application/` (`extract`, `summary`, `proxy`, `gates`)
- **Puertos/DOMINIO**: `internal/domain/` (interfaces, errores, tipos, eventos)
- **Adaptadores**: `internal/adapters/http_*`, `internal/adapters/redis_streams`
- **API**: `internal/api/` (gin + middlewares: recovery, correlation, access log, rate-limit, concurrency)
- **Composición**: `cmd/server/main.go` (fail-fast por env críticas, graceful shutdown)

Contratos clave:

- **API pública**: [`docs/contracts/openapi.yaml`](docs/contracts/openapi.yaml)
- **Workers (Extractor/Resumidor)**: [`docs/contracts/workers.md`](docs/contracts/workers.md)
- **Stream de eventos**: [`docs/contracts/events.md`](docs/contracts/events.md) (`original`, `summary_resolved`, campos en `payload` JSON + `document_id` como clave)
- **Delta Persistidor**: [`docs/contracts/delta-persister.md`](docs/contracts/delta-persister.md) (cambios reconocidos respecto a blueprint)

Decisiones de arquitectura: [`docs/adr/`](docs/adr/) (ADR-0001..0007). `ARCHITECTURE_ORCHESTRATOR.md` está excluido de git (solo disco).

---

## 8. Desarrollo Local / Tests

```bash
# Compilar, vet, tests y formato
go build ./...
go vet ./...
go test ./... -count=1
test -z "$(gofmt -l .)"

# Tests con verbosidad
go test -v ./internal/application ./internal/api ./internal/api/middleware
```

TDD estricto (rojo → verde). Sin comentarios innecesarios. Vocabulario según `CONTEXT.md`.

---

## 9. Limpieza

```bash
docker compose down -v  # detiene y borra volúmenes
docker compose -f compose.yml -f compose.mocks.yml down -v
docker network rm public-net mired 2>/dev/null || true
```

---

## Licencia

MIT. Ver [LICENSE](LICENSE).