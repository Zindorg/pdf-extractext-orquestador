package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// ExtractUseCase recibe un documento, lo valida, extrae su texto, resuelve el resumen si se pidió y
// publica los eventos en el stream.
type ExtractUseCase struct {
	extractor  domain.Extractor
	summarizer domain.Summarizer
	publisher  domain.EventPublisher
}

// NewExtractUseCase arma el caso de uso con sus tres colaboradores.
func NewExtractUseCase(extractor domain.Extractor, summarizer domain.Summarizer, publisher domain.EventPublisher) *ExtractUseCase {
	return &ExtractUseCase{extractor: extractor, summarizer: summarizer, publisher: publisher}
}

// ingestion agrupa lo ya resuelto de una ingesta —identidad, texto y resumen— para no arrastrar
// media docena de parámetros entre la publicación y el armado del resultado.
type ingestion struct {
	documentID    domain.DocumentID
	checksum      domain.Checksum
	extraction    domain.Extraction
	summary       domain.Summary
	summaryStatus domain.SummaryStatus
}

// Execute corre el pipeline de ingesta: puerta 1 → extraer → puerta 2 → resumen → publicar. El
// resumen se resuelve antes de publicar nada, para que un fallo del Resumidor no deje un documento
// a medias en el stream: si falla, no hay ninguna publicación y no hay nada que deshacer.
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

	summary, err := u.summarize(ctx, in.Summarize, extraction.Text)
	if err != nil {
		return domain.IngestResult{}, err
	}

	ing := ingestion{
		documentID:    domain.DocumentID(uuid.NewString()),
		checksum:      domain.NewChecksum(extraction.Text),
		extraction:    extraction,
		summary:       summary,
		summaryStatus: summaryStatus(in.Summarize),
	}

	if err := u.publish(ctx, ing); err != nil {
		return domain.IngestResult{}, err
	}

	return ing.result(in.Filename), nil
}

// summarize resuelve el resumen solo si el cliente lo pidió; si no, devuelve un resumen vacío.
func (u *ExtractUseCase) summarize(ctx context.Context, requested bool, text string) (domain.Summary, error) {
	if !requested {
		return domain.Summary{}, nil
	}
	return u.summarizer.Summarize(ctx, text)
}

// publish publica `original` y, solo si el resumen se pidió, `summary_resolved` a continuación. El
// orden es el contrato: el consumidor reintenta un `summary_resolved` que llegue huérfano.
func (u *ExtractUseCase) publish(ctx context.Context, ing ingestion) error {
	if err := u.publisher.Publish(ctx, domain.OriginalEvent{
		DocumentID:       ing.documentID,
		Checksum:         ing.checksum,
		ExtractedText:    ing.extraction.Text,
		MimeType:         mimeTypePDF,
		SummaryRequested: ing.summaryStatus == domain.SummaryResolved,
		SummaryStatus:    ing.summaryStatus,
		ExtractionTimeMS: ing.extraction.TimeMS,
		ExtractedAt:      time.Now().UTC(),
	}); err != nil {
		return err
	}
	if ing.summaryStatus != domain.SummaryResolved {
		return nil
	}
	return u.publisher.Publish(ctx, domain.SummaryResolvedEvent{
		DocumentID:    ing.documentID,
		Checksum:      ing.checksum,
		Summary:       ing.summary.Text,
		SummaryTimeMS: ing.summary.TimeMS,
		ResolvedAt:    time.Now().UTC(),
	})
}

// result arma la salida de dominio de la ingesta.
func (ing ingestion) result(filename string) domain.IngestResult {
	return domain.IngestResult{
		DocumentID:       ing.documentID,
		Filename:         filename,
		Checksum:         ing.checksum,
		DeliveryStatus:   domain.DeliveryIngested,
		SummaryStatus:    ing.summaryStatus,
		ExtractionTimeMS: ing.extraction.TimeMS,
		SummaryTimeMS:    ing.summary.TimeMS,
		SummaryText:      ing.summary.Text,
		Warnings:         ing.extraction.Warnings,
	}
}

// summaryStatus es el estado de resumen de una ingesta que llegó a publicar: pedido implica
// `RESOLVED`, no pedido implica `NOT_REQUESTED`.
func summaryStatus(requested bool) domain.SummaryStatus {
	if requested {
		return domain.SummaryResolved
	}
	return domain.SummaryNotRequested
}
