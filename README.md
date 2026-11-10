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
| `VERSION` | `dev` | Tag de imagen Docker y versión que se anuncia en Consul |
| `TZ` | `America/Argentina/Mendoza` | Zona horaria |
| `PUBLIC_HOST` | — (obligatoria en compose) | Host público usado por Traefik (router `Host`) |
| `PUBLIC_BASE_URL` | — (obligatoria) | URL absoluta http(s) para el `Location` de las respuestas |
| `EXTRACTOR_URL` / `SUMMARIZER_URL` / `PERSISTER_URL` | — (obligatorias) | URLs internas de los workers |
| `REDIS_URL` | — (obligatoria) | `redis://[:password@]host:port[/db]`. `REDIS_ADDR` (`host:port`) se sigue aceptando |
| `STREAM_NAME` | `document-events` | Stream Redis para eventos de ingesta |
| `STREAM_MAX_LEN` | `10000` | Tope aproximado del stream (`XADD MAXLEN ~`); `0` = sin tope |
| `MAX_DOCUMENT_BYTES` | `10485760` | Tamaño máximo del documento (10 MiB), en API y web |
| `EXTRACTOR_TIMEOUT` | `30s` | Timeout HTTP hacia el Extractor |
| `SUMMARIZER_TIMEOUT` | `120s` | Timeout HTTP hacia el Resumidor; también fija el `WriteTimeout` del servidor |
| `PERSISTER_TIMEOUT` | `15s` | Timeout HTTP hacia el Persistidor |
| `PERSISTER_MAX_RESPONSE_BYTES` | `33554432` | Tope de lo que se lee de una respuesta del Persistidor (32 MiB) |
| `EXTRACT_MAX_IN_FLIGHT` | `16` | Semáforo de concurrencia para `POST /api/v1/documents/extract` |
| `SUMMARY_MAX_IN_FLIGHT` | `4` | Semáforo de concurrencia para `POST /api/v1/documents/{id}/summary` |
| `RATE_LIMIT_PER_MINUTE` | `120` | Límite por IP de cliente y réplica; `0` lo desactiva |
| `TRUSTED_PROXIES` | — | CIDRs/IPs de los proxies (Traefik) cuyo `X-Forwarded-For` se acepta |
| `LOG_LEVEL` | `info` | Nivel de log (`debug/info/warn/error`) |
| `CONSUL_HTTP_ADDR` | — | API de Consul (`http://consul:8500`); vacía = no se registra |
| `CONSUL_HTTP_TOKEN` | — | Token de Consul, solo si el ACL lo exige |
| `CONSUL_SERVICE_NAME` | `pdf-extractext-orquestador` | Nombre del servicio en Consul |
| `SERVICE_ADDRESS` | hostname del contenedor | Dirección con la que Consul alcanza al servicio (y hace el check a `/ready`) |
| `EXTRACTOR_MOCK_DELAY_MS` | `0` | (Solo mocks) Demora artificial en Extractor mock (`lento.pdf`) |
| `SUMMARIZER_MOCK_DELAY_MS` | `0` | (Solo mocks) Demora artificial en Resumidor mock |

> Las URLs internas hacia workers (`extractor.internal`, `summarizer.internal`, `persister.internal`) y la dirección de Redis (`redis:6379`) están fijas en los composes (topología `mired`).

---

## 3. Estructura de despliegue

Cada pieza de infraestructura tiene su propio compose en `servicios/`; el orquestador usa el
`docker-compose.yml` de la raíz. Todos se comunican por redes Docker externas:

| Red | Quién | Para qué |
|---|---|---|
| `public-net` | `traefik-public`, orquestador | Borde HTTPS |
| `mired` | orquestador, `traefik-internal`, Redis, Consul, workers | Tráfico interno (`*.internal`, stream, registro) |
| `db-net` (interna) | MongoDB, Persistidor | Base de datos |

| Carpeta | Servicio | Antes de levantar |
|---|---|---|
| `servicios/consul/` | Consul (registro y KV), API solo en `127.0.0.1:8500` | `cp config/acl-tokens.json.example config/acl-tokens.json` y completar con `uuidgen` |
| `servicios/redis/` | Redis 7 con AOF, stream `document-events` | — |
| `servicios/mongodb/` | MongoDB 8 (lo usa el Persistidor) | `cp .env.example .env` con credenciales reales |
| `servicios/traefik/` | `traefik-public` (80/443) y `traefik-internal` | Certificados TLS: ver `certs/README.md` (mkcert) |

```bash
docker network create public-net
docker network create mired
docker network create --internal db-net
```

---

## 4. Despliegue con Docker Compose

```bash
(cd servicios/consul  && docker compose up -d)
(cd servicios/redis   && docker compose up -d)
(cd servicios/mongodb && docker compose up -d)
(cd servicios/traefik && docker compose up -d)

cp .env.example .env   # ajustar PUBLIC_HOST y PUBLIC_BASE_URL
docker compose up -d --build
```

El orquestador queda en `public-net` + `mired`, **sin puertos al host**. Al arrancar se registra en
Consul (check HTTP a `/ready`) y al apagarse se da de baja. Comprobar:

```bash
docker compose ps                                   # orchestrator: healthy
docker compose logs orchestrator | grep "registrado en Consul"
curl -s "http://localhost:8500/v1/health/service/pdf-extractext-orquestador?passing"
curl -sk https://${PUBLIC_HOST}/api/v1/health
```

Las sondas `/health`, `/healthz` y `/ready` no se publican hacia afuera: las usan Docker y Consul.

---

## 5. Smoke Local con Mocks

`compose.mocks.yml` agrega mocks de los tres workers detrás de `traefik-internal`, con los mismos
nombres internos que en producción. El mock del Persistidor consume el stream de verdad, así que el
camino publicación → persistencia → lectura se prueba completo. Requiere Redis y Traefik levantados
(sección 4).

```bash
docker compose -f docker-compose.yml -f compose.mocks.yml up -d --build
```

Probar una ingesta con resumen desde la red interna:

```bash
printf '%%PDF-1.7\n' > /tmp/contrato.pdf
docker run --rm --network mired -v /tmp/contrato.pdf:/tmp/contrato.pdf:ro \
  curlimages/curl:8.11.1 -s -X POST http://orchestrator:8080/api/v1/documents/extract \
  -F "document=@/tmp/contrato.pdf" -F "summarize=true"
```

Respuesta esperada (`202`): `document_id`, `delivery_status=INGESTED`, `summary_status=RESOLVED`,
`summary`, `processing_time_ms`, `extraction_time_ms`, `summary_time_ms`.

Casos de prueba (los decide el nombre del archivo en el Extractor mock):

| Archivo | Comportamiento |
|---|---|
| `contrato.pdf` | 200 success |
| `vacio.pdf` | Texto extraído vacío → puerta 2 rechaza (no se publica) |
| `cifrado.pdf` | 422 `encrypted_pdf` |
| `roto.pdf` | 422 `corrupt` |
| `escaneado.pdf` | 422 `unsupported_features` |
| `lento.pdf` | Demora (`EXTRACTOR_MOCK_DELAY_MS`) |

Resumidor mock: un texto que contenga `[demasiado-largo]` → 422 `document_too_long` (502 en la API
pública); con `[resumen-vacio]` → resumen vacío, rechazado por el caso de uso antes de publicar.

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

Mapeo código→HTTP (tabla única en `internal/httpstatus`, compartida por la API y la web): `invalid_request`→400, `document_not_found`/`summary_not_available`→404, `summary_pending`/`summary_already_resolved`/`checksum_conflict`→409, `payload_too_large`→413, `unsupported_media_type`→415, `extraction_failed`→422, `rate_limited`/`concurrency_limited`→429 (+`Retry-After`), `summary_failed`→502, `extractor_unavailable`/`persister_unavailable`→503, `internal`→500.

---

## 7. Arquitectura y Contratos

- **Casos de uso**: `internal/application/` (`extract`, `summary`, `proxy`, `gates`)
- **Puertos/DOMINIO**: `internal/domain/` (interfaces, errores, tipos, eventos)
- **Adaptadores**: `internal/adapters/http*`, `internal/adapters/redisstreams`, `internal/adapters/consul`
- **API**: `internal/api/` (gin + middlewares: correlation, access log, recovery, rate-limit, concurrency)
- **Web**: `internal/web/` (páginas HTML server-side sobre los mismos casos de uso)
- **Errores HTTP**: `internal/httpstatus/` (tabla única código → status)
- **Composición**: `cmd/server/main.go` (fail-fast por env críticas, graceful shutdown)

Contratos clave:

- **API pública**: [`docs/contracts/openapi.yaml`](docs/contracts/openapi.yaml)
- **Workers (Extractor/Resumidor)**: [`docs/contracts/workers.md`](docs/contracts/workers.md)
- **Stream de eventos**: [`docs/contracts/events.md`](docs/contracts/events.md) (`original`, `summary_resolved`, campos en `payload` JSON + `document_id` como clave)
- **Delta Persistidor**: [`docs/contracts/delta-persister.md`](docs/contracts/delta-persister.md) (cambios reconocidos respecto a blueprint)

Decisiones de arquitectura: [`docs/adr/`](docs/adr/) (ADR-0001..0008). `ARCHITECTURE_ORCHESTRATOR.md` está excluido de git (solo disco).

---

## 8. Desarrollo Local / Tests

Los mismos chequeos que corre CI (`.github/workflows/ci.yml`):

```bash
test -z "$(gofmt -l cmd internal mocks)"
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 -checks all,-ST1000 ./...
go test -race -count=1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# Tests con verbosidad
go test -v ./internal/application ./internal/api ./internal/api/middleware
```

TDD estricto (rojo → verde). Sin comentarios innecesarios. Vocabulario según `CONTEXT.md`.

---

## 9. Limpieza

```bash
docker compose -f docker-compose.yml -f compose.mocks.yml down
for dir in servicios/*/; do (cd "$dir" && docker compose down); done
docker network rm public-net mired db-net 2>/dev/null || true
# Agregar -v a cada `down` borra también los volúmenes (datos de Redis, Mongo y Consul).
```

---

## Licencia

MIT. Ver [LICENSE](LICENSE).
