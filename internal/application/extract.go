package application

import (
	"context"
	"io"
	"log/slog"
	"strings"
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
	persister  domain.PersisterProxy

	maxDocumentBytes int
	now              func() time.Time
	logger           *slog.Logger
}

// NewExtractUseCase arma el caso de uso con sus cuatro colaboradores y el tope de tamaño por defecto.
func NewExtractUseCase(extractor domain.Extractor, summarizer domain.Summarizer, publisher domain.EventPublisher, persister domain.PersisterProxy) *ExtractUseCase {
	return &ExtractUseCase{
		extractor:        extractor,
		summarizer:       summarizer,
		publisher:        publisher,
		persister:        persister,
		maxDocumentBytes: DefaultMaxDocumentBytes,
		now:              time.Now,
		logger:           slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

// WithLogger fija el logger para lo que el caso de uso resuelve sin fallar la petición (por
// ejemplo, una búsqueda de duplicados que no respondió).
func (u *ExtractUseCase) WithLogger(logger *slog.Logger) *ExtractUseCase {
	if logger != nil {
		u.logger = logger
	}
	return u
}

// WithMaxDocumentBytes fija el tope de tamaño del documento (MAX_DOCUMENT_BYTES). Un valor no
// positivo conserva el default.
func (u *ExtractUseCase) WithMaxDocumentBytes(n int) *ExtractUseCase {
	if n > 0 {
		u.maxDocumentBytes = n
	}
	return u
}

// ingestion agrupa lo ya resuelto de una ingesta —identidad, texto y resumen— para no arrastrar
// media docena de parámetros entre la publicación y el armado del resultado.
type ingestion struct {
	documentID    domain.DocumentID
	checksum      domain.Checksum
	extraction    domain.Extraction
	summary       domain.Summary
	summaryStatus domain.SummaryStatus
	mimeType      string
	summarize     bool
	extractedAt   time.Time
	filename      string
	metadata      map[string]any
}

// Execute corre el pipeline de ingesta: puerta 1 → extraer → puerta 2 → resumen → publicar. El
// resumen se resuelve antes de publicar nada, para que un fallo del Resumidor no deje un documento
// a medias en el stream: si falla, no hay ninguna publicación y no hay nada que deshacer.
func (u *ExtractUseCase) Execute(ctx context.Context, in domain.IncomingDocument) (domain.IngestResult, error) {
	if err := validateDocument(len(in.Bytes), in.Bytes, u.maxDocumentBytes); err != nil {
		return domain.IngestResult{}, err
	}

	documentID := domain.DocumentID(uuid.NewString())

	extraction, err := u.extractor.Extract(ctx, documentID, in)
	if err != nil {
		return domain.IngestResult{}, err
	}
	if err := classifyExtractionReason(extraction.Reason); err != nil {
		return domain.IngestResult{}, err
	}

	checksum := domain.NewChecksum(extraction.Text)
	if existing, found := u.findDuplicate(ctx, documentID, checksum); found {
		return u.resolveDuplicate(ctx, in, existing, checksum, extraction)
	}

	summary, err := u.summarize(ctx, in.Summarize, documentID, extraction.Text)
	if err != nil {
		return domain.IngestResult{}, err
	}

	ing := ingestion{
		documentID:    documentID,
		checksum:      checksum,
		extraction:    extraction,
		summary:       summary,
		summaryStatus: summaryStatus(in.Summarize),
		mimeType:      mimeTypePDF,
		summarize:     in.Summarize,
		extractedAt:   u.now().UTC(),
		filename:      in.Filename,
		metadata:      in.Metadata,
	}

	if err := u.publish(ctx, ing); err != nil {
		return domain.IngestResult{}, err
	}

	return ing.result(in.Filename), nil
}

// findDuplicate busca un documento activo con el mismo contenido. Es una optimización, no la
// garantía: el lookup no es atómico con la publicación y la garantía real es el índice único parcial
// del Persistidor (events.md). Por eso, si la consulta falla, la ingesta sigue como si no hubiera
// duplicado y el error queda en el log.
func (u *ExtractUseCase) findDuplicate(ctx context.Context, documentID domain.DocumentID, checksum domain.Checksum) (domain.StoredDocumentSummary, bool) {
	existing, found, err := u.persister.FindActiveByChecksum(ctx, checksum)
	if err != nil {
		u.logger.Warn("no se pudo consultar duplicados; se ingiere como documento nuevo",
			"document_id", documentID, "checksum", checksum, "error", err)
		return domain.StoredDocumentSummary{}, false
	}
	return existing, found
}

// resolveDuplicate responde con el documento que ya existe. Si el cliente pidió resumen y el
// existente no lo tiene ni lo está esperando (NOT_REQUESTED o FAILED), lo resume ahora y publica
// `summary_resolved` sobre ese documento: subir de nuevo un PDF pidiendo resumen no debe ignorar el
// pedido solo porque el contenido ya estaba guardado.
func (u *ExtractUseCase) resolveDuplicate(ctx context.Context, in domain.IncomingDocument, existing domain.StoredDocumentSummary, checksum domain.Checksum, extraction domain.Extraction) (domain.IngestResult, error) {
	result := domain.IngestResult{
		DocumentID:       existing.DocumentID,
		Filename:         in.Filename,
		Checksum:         checksum,
		DeliveryStatus:   domain.DeliveryAvailable,
		SummaryStatus:    existing.SummaryStatus,
		SummaryText:      existing.Summary,
		ExtractionTimeMS: extraction.TimeMS,
		Warnings:         extraction.Warnings,
	}
	needsSummary := existing.SummaryStatus == domain.SummaryNotRequested || existing.SummaryStatus == domain.SummaryFailed
	if !in.Summarize || !needsSummary {
		return result, nil
	}

	summary, err := u.summarize(ctx, true, existing.DocumentID, extraction.Text)
	if err != nil {
		return domain.IngestResult{}, err
	}
	if err := u.publisher.Publish(ctx, domain.SummaryResolvedEvent{
		DocumentID:    existing.DocumentID,
		Checksum:      checksum,
		Summary:       summary.Text,
		SummaryTimeMS: summary.TimeMS,
		ResolvedAt:    u.now().UTC(),
	}); err != nil {
		return domain.IngestResult{}, err
	}
	result.SummaryStatus = domain.SummaryResolved
	result.SummaryText = summary.Text
	result.SummaryTimeMS = summary.TimeMS
	return result, nil
}

// summarize resuelve el resumen solo si el cliente lo pidió; si no, devuelve un resumen vacío. Un
// resumen sin contenido utilizable se trata como fallo del Resumidor: el contrato no admite
// `summary` vacío (events.md).
func (u *ExtractUseCase) summarize(ctx context.Context, requested bool, documentID domain.DocumentID, text string) (domain.Summary, error) {
	if !requested {
		return domain.Summary{}, nil
	}
	summary, err := u.summarizer.Summarize(ctx, documentID, text)
	if err != nil {
		return domain.Summary{}, err
	}
	if strings.TrimSpace(summary.Text) == "" {
		return domain.Summary{}, domain.ErrSummaryFailed
	}
	return summary, nil
}

// publish publica `original` y, solo si el resumen se pidió, `summary_resolved` a continuación. El
// orden es el contrato: el consumidor reintenta un `summary_resolved` que llegue huérfano.
func (u *ExtractUseCase) publish(ctx context.Context, ing ingestion) error {
	if err := u.publisher.Publish(ctx, domain.OriginalEvent{
		DocumentID:       ing.documentID,
		Checksum:         ing.checksum,
		ExtractedText:    ing.extraction.Text,
		MimeType:         ing.mimeType,
		SummaryRequested: ing.summarize,
		SummaryStatus:    ing.summaryStatus,
		ExtractionTimeMS: ing.extraction.TimeMS,
		ExtractedAt:      ing.extractedAt,
		Filename:         ing.filename,
		Metadata:         ing.metadata,
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
		ResolvedAt:    u.now().UTC(),
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
