package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// ExtractUseCase recibe un documento, lo valida, extrae su texto y lo publica en el stream.
type ExtractUseCase struct {
	extractor domain.Extractor
	publisher domain.EventPublisher
}

// NewExtractUseCase arma el caso de uso con el Extractor y el publicador de eventos.
func NewExtractUseCase(extractor domain.Extractor, publisher domain.EventPublisher) *ExtractUseCase {
	return &ExtractUseCase{extractor: extractor, publisher: publisher}
}

// Execute corre el pipeline de ingesta: puerta 1 → extraer → puerta 2 → checksum → publicar. La
// respuesta no espera a que el documento sea consultable: publicar en el stream es lo único que lo
// hace existir en el sistema.
func (u *ExtractUseCase) Execute(ctx context.Context, in domain.IncomingDocument) (domain.IngestResult, error) {
	if err := validateDocument(len(in.Bytes), in.Bytes); err != nil {
		return domain.IngestResult{}, err
	}

	extraction, err := u.extractor.Extract(ctx, in)
	if err != nil {
		return domain.IngestResult{}, err
	}
	if err := classifyExtractionReason(extraction.Reason); err != nil {
		return domain.IngestResult{}, err
	}

	documentID := domain.DocumentID(uuid.NewString())
	checksum := domain.NewChecksum(extraction.Text)

	event := domain.OriginalEvent{
		DocumentID:       documentID,
		Checksum:         checksum,
		ExtractedText:    extraction.Text,
		MimeType:         mimeTypePDF,
		SummaryRequested: in.Summarize,
		SummaryStatus:    domain.SummaryNotRequested,
		ExtractionTimeMS: extraction.TimeMS,
		ExtractedAt:      time.Now().UTC(),
	}
	if err := u.publisher.Publish(ctx, event); err != nil {
		return domain.IngestResult{}, err
	}

	return domain.IngestResult{
		DocumentID:       documentID,
		Filename:         in.Filename,
		Checksum:         checksum,
		DeliveryStatus:   domain.DeliveryIngested,
		SummaryStatus:    domain.SummaryNotRequested,
		ExtractionTimeMS: extraction.TimeMS,
		Warnings:         extraction.Warnings,
	}, nil
}
