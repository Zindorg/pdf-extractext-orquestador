# Contratos con los workers

> **PROPUESTA v0 — fecha: 2026-10-07.** Los servicios de extracción y de resumen todavía no tienen
> repositorio. Estos contratos son la propuesta del Orquestador; tienen que confirmarlos quienes
> implementen cada worker (ver §Puntos a negociar). Los tests de contrato de este repo son los que los
> mantienen honestos.

Son los contratos **de salida** del Orquestador: los pide el Orquestador a dos servicios que no conoce más
allá de esta página. No son la API pública (esa es `docs/contracts/openapi.yaml`); no tienen red externa.

## Contrato común

- **Transporte:** HTTP/1.1, JSON utf-8. Sin gRPC ni streaming binario obligatorio: el texto extraído ya es
  texto, no hace falta un canal binario (ADR-0002). El Orquestador es el único cliente.
- **Identificación:** el Orquestador no envía identificador propio en el cuerpo. El `document_id` no existe
  aún en la llamada al Extractor (se genera antes), y no le dice nada al Resumidor: es una ida y vuelta
  entre dos procesos sin estado compartido.
- **Errores:** cualquier estado no `2xx` se traduce a un motivo. Si el motivo no se puede leer o no está en
  el enum, el Orquestador lo trata como un fallo del worker (ver §Extractor y §Resumidor).
- **Reintentos:** 2 reintentos ante error de **red** (timeout, conexión rechazada, corte), con espera
  creciente. **Cero reintentos ante un error de validación** (`4xx`): re-enviar el mismo payload va a fallar
  igual, y reintentar no es gratis (ocupa la puerta de concurrencia).
- **Timeouts:** el del Extractor es acotado (misma escala que la extracción); el del Resumidor es holgado,
  porque el modelo local es serializado (se explica en `internal/adapters/http_summarizer/client.go`).

## Extractor

Servicio que convierte un documento en su texto plano. Es la única etapa obligatoria del pipeline (ADR-0001,
paso 3).

### `POST /extract`

**Request** — `multipart/form-data`:

| Campo | Tipo | Descripción |
|---|---|---|
| `document` | binary (stream) | El archivo recibido, reenviado sin tocar el disco (Zero-Disk). El cuerpo se reusa tal cual; no se parsea. |
| `mime_type` | string | El tipo MIME declarado por el cliente, ya validado contra el formato del MVP (solo PDF, ADR-0008). |

**Response `200`** — `application/json`:

| Campo | Tipo | Descripción |
|---|---|---|
| `extracted_text` | string | El texto extraído. Es el contenido; de aquí sale el checksum (ADR-0003). |
| `extraction_time_ms` | entero | Duración de la extracción. Es una pata del total de la ingesta, no el total. |
| `warnings` | array[`{code, message}`] | Problemas no fatales (ej. texto extraído parcialmente). Vocabulario **abierto**. |

**Response `422`** — `application/json`:

| Campo | Tipo | Descripción |
|---|---|---|
| `reason` | string | Un valor del enum de motivos de extracción (abajo). |
| `message` | string | Detalle legible. |

### Enum de motivos de extracción

Lo consume la **puerta 2** (ADR-0001, paso 4): clasifica el resultado del Extractor y decide si se continúa o
se responde `422`. Se define una sola vez, acá, y se redefine solo si el Extractor real publica motivos
distintos.

| `reason` | Significa | Puerta 2 |
|---|---|---|
| `success` | Texto extraído completo. Se continúa. | admitir |
| `empty` | Texto extraído vacío, sin fallo de extracción. | `422`, no se publica nada |
| `encrypted_pdf` | El PDF está cifrado y no se pudo abrir. | `422`, no se publica nada |
| `password_protected` | El PDF pide contraseña. | `422`, no se publica nada |
| `corrupt` | El PDF está dañado y no se pudo leer. | `422`, no se publica nada |
| `unsupported_features` | El PDF se leyó pero tiene características no soportadas (páginas escaneadas sin OCR, fuentes embebidas inválidas). | `422`, no se publica nada |

Un `reason` fuera del enum no se traduce en `422`: se trata como fallo del Extractor → `503` (`extractor_unavailable`),
para no inventar una categoría que el servicio no declaró.

## Resumidor

Servicio que sintetiza en lenguaje natural el texto extraído. Es la etapa opcional (ADR-0001, paso 6). No
ve el documento; ve solo texto.

### `POST /summarize`

**Request** — `application/json`:

| Campo | Tipo | Descripción |
|---|---|---|
| `document_id` | string (UUID) | El identificador de la ingesta. Solo para correlación; el Resumidor no lo persiste. |
| `extracted_text` | string | El texto extraído, tal como salió del Extractor y del que se calculó el checksum. |

**Response `200`** — `application/json`:

| Campo | Tipo | Descripción |
|---|---|---|
| `summary` | string | La síntesis en lenguaje natural. |
| `summary_time_ms` | entero | Duración del resumen. Es la segunda pata del total. |

**Response `422`** — `application/json`:

| Campo | Tipo | Descripción |
|---|---|---|
| `reason` | string | Único motivo definido hoy: `document_too_long`. El texto supera la ventana de contexto del modelo. |
| `message` | string | Detalle legible. |

Un `422` con `document_too_long` es respuesta `502` (`summary_failed`) en la API pública: el resumen se pidió
y falló, y por la regla de publicar solo al final **no se ingirió nada** (ADR-0001). No es un error del
documento, así que no es `422` en la API; es un recurso (el Resumidor) que no pudo con el texto.

## Puntos a negociar

Lo que depende de la implementación real de cada worker, y que **no** se puede cerrar desde este repo:

1. **Los motivos del Extractor.** El enum de arriba es una propuesta. El Extractor real puede tener otros
   modos de fallo (ej. límite interno de tamaño, OCR ausente). Si cambia, cambia la puerta 2 y el
   `details.reason` de la API (Límite 2 de `tasks/plan.md`).
2. **El techo de 10 MiB.** La puerta 1 lo aplica en el borde, antes de llamar al Extractor. Si el Extractor
   tiene un límite propio menor, coordinar.
3. **Formato del cuerpo del Extractor.** Sin servidor de referencia, el shape de `multipart/form-data` es
   la convención mínima; el `mime_type` extra es el campo que permite que el Extractor decida sin parsear
   el archivo.
4. **`document_too_long` como único motivo del Resumidor.** Pueden aparecer más (ej. texto en idioma no
   soportado); se agregan al enum del Resumidor, que es abierto en la práctica aunque cerrado en este
   contrato.
5. **`document_id` en el Resumidor.** Si el Resumidor quiere persistir una traza de qué pidió, necesita
   saberlo; sino se quita. Es un campo de correlación, no de negocio.