# Mapa de Contextos

> Los contextos del ecosistema de extracción de documentos, dónde vive cada uno y cómo se comunican.
> El glosario de cada contexto está en su propio `CONTEXT.md`.

## Contextos

| Contexto | Servicio | Dónde vive | Responsabilidad |
|---|---|---|---|
| Orquestación | `orchestrator` | este repo | Punto de entrada público y coordinación del flujo. Es el único que conoce a los demás. |
| Extracción | `extractor` | repo externo | Obtiene el texto a partir del archivo, en memoria. |
| Resumen | `summarizer` | repo externo | Genera el resumen del texto extraído. Opcional. |
| Persistencia | `persister` | repo externo | Ingesta, almacenamiento y ciclo de vida del documento. Único con acceso a la base de datos. |

## Relaciones

- **Orquestación → Extracción**: HTTP, `POST /extract`. Consume el archivo en memoria y devuelve el texto extraído con el motivo si no es legible.
- **Orquestación → Resumen**: HTTP, `POST /summarize`. Se invoca solo si el cliente pidió resumen.
- **Orquestación → Redis Streams**: publica los eventos de ingesta. Único productor del stream.
- **Orquestación → Persistencia**: HTTP, solo lectura y borrado. La escritura no va por HTTP.
- **Redis Streams → Persistencia**: consumo asíncrono. Único consumidor del stream.
- **Los tres workers no se conocen entre sí**: no hay bus compartido ni llamadas directas. Regla hub & spoke.

## Redes

| Red | Quién | Exposición |
|---|---|---|
| `public-net` | `traefik-public`, `orchestrator` | El cliente llega acá. `traefik-public` termina TLS y aplica los límites del borde. |
| `mired` | `traefik-internal`, `orchestrator`, `extractor`, `summarizer`, `persister`, `redis` | Interna. El tráfico entre el Orquestador y los workers pasa por `traefik-internal` (ADR-0005). |
| `db-net` | `persister`, `mongodb` | Interna y sin salida a internet. |

El Orquestador llama a los workers por hostname interno (`extractor.internal`, `summarizer.internal`,
`persister.internal`), que resuelve a `traefik-internal` por alias de red. El puerto HTTP del Orquestador no se
publica al host: sólo `traefik-public` publica `80`/`443` (ADR-0005).
