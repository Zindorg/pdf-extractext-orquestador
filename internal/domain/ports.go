package domain

import "context"

// Extractor convierte un documento en su texto plano. El adaptador HTTP traduce la respuesta del
// servicio: un 200 a una Extraction con Reason `success`, y un 422 a una Extraction con el motivo
// del rechazo. Un fallo de transporte se devuelve como error.
type Extractor interface {
	Extract(ctx context.Context, in IncomingDocument) (Extraction, error)
}

// EventPublisher publica un evento en el stream. Los campos de transporte (`schema_version`,
// `event_type`) los agrega el adaptador.
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}
