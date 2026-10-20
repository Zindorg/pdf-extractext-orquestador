package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// SummaryUseCase resuelve el resumen de un documento que ya está persistido. Es el paso que el
// cliente pide después de una ingesta sin resumen: lee el documento por proxy, baja su texto
// extraído, resume, publica `summary_resolved` y devuelve el resultado.
type SummaryUseCase struct {
	persister  domain.PersisterProxy
	summarizer domain.Summarizer
	publisher  domain.EventPublisher
	now        func() time.Time
}

func NewSummaryUseCase(persister domain.PersisterProxy, summarizer domain.Summarizer, publisher domain.EventPublisher) *SummaryUseCase {
	return &SummaryUseCase{
		persister:  persister,
		summarizer: summarizer,
		publisher:  publisher,
		now:        time.Now,
	}
}

// documentView es la parte de la respuesta del Persistidor que este caso de uso interpreta. El resto
// de los campos no importan acá: la lectura es del Persistidor y el dominio no la modela entera
// (openapi.yaml, esquema `Document`).
type documentView struct {
	SummaryStatus domain.SummaryStatus `json:"summary_status"`
}

// Execute resuelve el resumen a demanda. Devuelve `ErrSummaryAlreadyResolved` si el documento ya
// tiene resumen —no se vuelve a pedir— y `ErrSummaryPending` si está en camino —el cliente espera y
// relee—; en ninguno de los dos casos toca al Resumidor ni publica nada. El 409 previo es una guarda
// de defensa, no la única protección: el Persistidor descarta en silencio un `summary_resolved` sobre
// un documento ya completo (ARCHITECTURE_REPOSITORIES.md §4).
func (uc *SummaryUseCase) Execute(ctx context.Context, documentID domain.DocumentID) (domain.SummaryRequestResult, error) {
	startedAt := uc.now()

	view, err := uc.readDocument(ctx, documentID)
	if err != nil {
		return domain.SummaryRequestResult{}, err
	}
	switch view.SummaryStatus {
	case domain.SummaryResolved:
		return domain.SummaryRequestResult{}, domain.ErrSummaryAlreadyResolved
	case domain.SummaryPending:
		return domain.SummaryRequestResult{}, domain.ErrSummaryPending
	}

	text, err := uc.downloadText(ctx, documentID)
	if err != nil {
		return domain.SummaryRequestResult{}, err
	}
	if strings.TrimSpace(text) == "" {
		return domain.SummaryRequestResult{}, domain.ErrSummaryFailed
	}

	summary, err := uc.summarizer.Summarize(ctx, documentID, text)
	if err != nil {
		return domain.SummaryRequestResult{}, err
	}
	if strings.TrimSpace(summary.Text) == "" {
		return domain.SummaryRequestResult{}, domain.ErrSummaryFailed
	}

	err = uc.publisher.Publish(ctx, domain.SummaryResolvedEvent{
		DocumentID:    documentID,
		Checksum:      domain.NewChecksum(text),
		Summary:       summary.Text,
		SummaryTimeMS: summary.TimeMS,
		ResolvedAt:    uc.now(),
	})
	if err != nil {
		return domain.SummaryRequestResult{}, err
	}

	return domain.SummaryRequestResult{
		DocumentID:       documentID,
		Summary:          summary.Text,
		SummaryTimeMS:    summary.TimeMS,
		ProcessingTimeMS: uc.now().Sub(startedAt).Milliseconds(),
	}, nil
}

func (uc *SummaryUseCase) readDocument(ctx context.Context, documentID domain.DocumentID) (documentView, error) {
	resp, err := uc.persister.Do(ctx, domain.PersisterRequest{
		Method: "GET",
		Path:   listPath + "/" + string(documentID),
	})
	if err != nil {
		return documentView{}, err
	}

	var view documentView
	if err := json.Unmarshal(resp.Body, &view); err != nil {
		return documentView{}, err
	}
	return view, nil
}

func (uc *SummaryUseCase) downloadText(ctx context.Context, documentID domain.DocumentID) (string, error) {
	resp, err := uc.persister.Do(ctx, domain.PersisterRequest{
		Method: "GET",
		Path:   listPath + "/" + string(documentID) + "/download/original",
	})
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}
