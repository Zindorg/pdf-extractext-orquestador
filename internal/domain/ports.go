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
