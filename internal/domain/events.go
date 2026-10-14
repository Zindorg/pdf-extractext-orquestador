package domain

import "time"

// Event es cualquier evento que el Orquestador publica en el stream (events.md). Es una unión
// sellada: los campos que solo existen en el transporte (`schema_version`, `event_type`) los
// agrega el adaptador, no el dominio.
type Event interface {
	isEvent()
}

// OriginalEvent transporta el texto extraído de un documento. Se publica después de que el
// resumen resolvió —o de que se decidió no pedirlo—, nunca antes.
type OriginalEvent struct {
	DocumentID       DocumentID
	Checksum         Checksum
	ExtractedText    string
	MimeType         string
	SummaryRequested bool
	SummaryStatus    SummaryStatus
	ExtractionTimeMS int64
	ExtractedAt      time.Time
}

func (OriginalEvent) isEvent() {}

// SummaryResolvedEvent transporta el resumen ya calculado. Solo existe si el cliente pidió resumen.
type SummaryResolvedEvent struct {
	DocumentID    DocumentID
	Checksum      Checksum
	Summary       string
	SummaryTimeMS int64
	ResolvedAt    time.Time
}

func (SummaryResolvedEvent) isEvent() {}
