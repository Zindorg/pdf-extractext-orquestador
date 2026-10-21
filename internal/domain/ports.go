package domain

import "context"

// Extractor convierte un documento en su texto plano. El adaptador HTTP traduce la respuesta del
// servicio: un 200 a una Extraction con Reason `success`, y un 422 a una Extraction con el motivo
// del rechazo. Un fallo de transporte se devuelve como error.
//
// documentID no lo usa el Extractor para extraer: viaja solo para correlacionar sus logs con la
// ingesta (ADR-0004). Es el único correlador del sistema.
type Extractor interface {
	Extract(ctx context.Context, documentID DocumentID, in IncomingDocument) (Extraction, error)
}

// Summarizer sintetiza en lenguaje natural el texto extraído. Un resultado o un error suyo hacen
// fallar la ingesta completa: el resumen se resuelve antes de publicar nada.
//
// documentID no lo usa el Resumidor para resumir: viaja solo para correlacionar sus logs con la
// ingesta (ADR-0004).
type Summarizer interface {
	Summarize(ctx context.Context, documentID DocumentID, text string) (Summary, error)
}

// EventPublisher publica un evento en el stream. Los campos de transporte (`schema_version`,
// `event_type`) los agrega el adaptador.
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}

// PersisterRequest es una petición de lectura o borrado hacia el Persistidor. `Path` y `RawQuery`
// van tal como los declaró el cliente, sin reescribir el prefijo `/api/v1` (ADR-0005, delta
// #10): la allowlist de rutas vive en el caso de uso, no acá.
type PersisterRequest struct {
	Method   string
	Path     string
	RawQuery string
}

// PersisterResponse es la respuesta cruda del Persistidor: el proxy es transparente, así que status,
// cabeceras y cuerpo vuelven al cliente sin interpretar. La única lectura que se hace del cuerpo es
// para traducir un error (un `4xx` no se reenvía tal cual).
type PersisterResponse struct {
	StatusCode int
	Header     map[string][]string
	Body       []byte
}

// StoredDocumentSummary es el resumen del documento activo devuelto por la consulta por checksum.
type StoredDocumentSummary struct {
	DocumentID    DocumentID
	SummaryStatus SummaryStatus
	Summary       string
}

// PersisterProxy reenvía peticiones al Persistidor. Un `4xx` que el Persistidor reporta con un
// código del vocabulario público sale como error de dominio tipado; un fallo de transporte o un
// estado no previsto sale como error sin tipar y la capa api lo responde `500`.
type PersisterProxy interface {
	Do(ctx context.Context, req PersisterRequest) (PersisterResponse, error)
	FindActiveByChecksum(ctx context.Context, checksum Checksum) (StoredDocumentSummary, bool, error)
}
