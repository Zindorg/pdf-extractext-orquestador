package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

const validPDFBytes = "%PDF-1.7\ntexto del documento"

type extractorStub struct {
	called     bool
	documentID domain.DocumentID
	result     domain.Extraction
	err        error
}

func (s *extractorStub) Extract(_ context.Context, documentID domain.DocumentID, _ domain.IncomingDocument) (domain.Extraction, error) {
	s.called = true
	s.documentID = documentID
	return s.result, s.err
}

type summarizerSpy struct {
	called     bool
	documentID domain.DocumentID
	result     domain.Summary
	err        error
}

func (s *summarizerSpy) Summarize(_ context.Context, documentID domain.DocumentID, _ string) (domain.Summary, error) {
	s.called = true
	s.documentID = documentID
	return s.result, s.err
}

type publisherSpy struct {
	events []domain.Event
	err    error
}

func (s *publisherSpy) Publish(_ context.Context, event domain.Event) error {
	s.events = append(s.events, event)
	return s.err
}

func TestExtractUseCasePublishesOriginalWithoutSummary(t *testing.T) {
	extractor := &extractorStub{result: domain.Extraction{
		Text:   "texto plano",
		TimeMS: 1240,
		Reason: domain.ReasonSuccess,
	}}
	summarizer := &summarizerSpy{}
	publisher := &publisherSpy{}

	result, err := NewExtractUseCase(extractor, summarizer, publisher).Execute(context.Background(), domain.IncomingDocument{
		Bytes:     []byte(validPDFBytes),
		Filename:  "doc.pdf",
		Summarize: false,
	})
	if err != nil {
		t.Fatalf("Execute() = %v, quería nil", err)
	}

	if result.DeliveryStatus != domain.DeliveryIngested {
		t.Fatalf("DeliveryStatus = %q, quería INGESTED", result.DeliveryStatus)
	}
	if result.SummaryStatus != domain.SummaryNotRequested {
		t.Fatalf("SummaryStatus = %q, quería NOT_REQUESTED", result.SummaryStatus)
	}
	if _, err := uuid.Parse(string(result.DocumentID)); err != nil {
		t.Fatalf("DocumentID = %q, no es un UUID válido: %v", result.DocumentID, err)
	}
	if extractor.documentID != result.DocumentID {
		t.Fatalf("el Extractor recibió document_id %q, quería %q", extractor.documentID, result.DocumentID)
	}
	if summarizer.called {
		t.Fatal("llamó al Resumidor con summarize=false")
	}

	if len(publisher.events) != 1 {
		t.Fatalf("publicó %d eventos, quería 1", len(publisher.events))
	}
	original, ok := publisher.events[0].(domain.OriginalEvent)
	if !ok {
		t.Fatalf("evento = %T, quería domain.OriginalEvent", publisher.events[0])
	}
	if original.DocumentID != result.DocumentID {
		t.Fatalf("evento.DocumentID = %q, quería %q", original.DocumentID, result.DocumentID)
	}
	if original.Checksum != domain.NewChecksum("texto plano") {
		t.Fatalf("evento.Checksum = %q, quería %q", original.Checksum, domain.NewChecksum("texto plano"))
	}
	if original.SummaryRequested || original.SummaryStatus != domain.SummaryNotRequested {
		t.Fatalf("resumen en el evento = requested=%v status=%q, quería false/NOT_REQUESTED", original.SummaryRequested, original.SummaryStatus)
	}
	if original.ExtractedAt.IsZero() {
		t.Fatal("el evento no lleva extracted_at")
	}
}

func TestExtractUseCasePublishesSummaryAfterOriginal(t *testing.T) {
	extractor := &extractorStub{result: domain.Extraction{
		Text:   "texto plano",
		TimeMS: 1240,
		Reason: domain.ReasonSuccess,
	}}
	summarizer := &summarizerSpy{result: domain.Summary{Text: "resumen", TimeMS: 8300}}
	publisher := &publisherSpy{}

	result, err := NewExtractUseCase(extractor, summarizer, publisher).Execute(context.Background(), domain.IncomingDocument{
		Bytes:     []byte(validPDFBytes),
		Filename:  "doc.pdf",
		Summarize: true,
	})
	if err != nil {
		t.Fatalf("Execute() = %v, quería nil", err)
	}

	if result.SummaryStatus != domain.SummaryResolved {
		t.Fatalf("SummaryStatus = %q, quería RESOLVED", result.SummaryStatus)
	}
	if result.SummaryText != "resumen" || result.SummaryTimeMS != 8300 {
		t.Fatalf("resumen en el resultado = %q/%d, quería resumen/8300", result.SummaryText, result.SummaryTimeMS)
	}
	if extractor.documentID != result.DocumentID || summarizer.documentID != result.DocumentID {
		t.Fatalf("document_id desparejo entre workers y resultado: extractor=%q resumidor=%q resultado=%q",
			extractor.documentID, summarizer.documentID, result.DocumentID)
	}

	if len(publisher.events) != 2 {
		t.Fatalf("publicó %d eventos, quería 2 (original y summary_resolved)", len(publisher.events))
	}
	original, ok := publisher.events[0].(domain.OriginalEvent)
	if !ok {
		t.Fatalf("primer evento = %T, quería domain.OriginalEvent", publisher.events[0])
	}
	if original.SummaryStatus != domain.SummaryResolved || !original.SummaryRequested {
		t.Fatalf("resumen en original = %q/%v, quería RESOLVED/true", original.SummaryStatus, original.SummaryRequested)
	}
	resolved, ok := publisher.events[1].(domain.SummaryResolvedEvent)
	if !ok {
		t.Fatalf("segundo evento = %T, quería domain.SummaryResolvedEvent", publisher.events[1])
	}
	if resolved.Summary != "resumen" || resolved.DocumentID != result.DocumentID || resolved.Checksum != domain.NewChecksum("texto plano") {
		t.Fatalf("summary_resolved = %+v, no coincide con la ingesta", resolved)
	}
}

func TestExtractUseCaseRejectsBeforeExtracting(t *testing.T) {
	oversize := make([]byte, maxDocumentBytes+1)
	copy(oversize, "%PDF-")

	cases := []struct {
		name string
		body []byte
		want error
	}{
		{"sin firma pdf", []byte("PK\x03\x04 nada"), domain.ErrUnsupportedMediaType},
		{"por encima del tope", oversize, domain.ErrDocumentTooLarge},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extractor := &extractorStub{}
			publisher := &publisherSpy{}

			_, err := NewExtractUseCase(extractor, &summarizerSpy{}, publisher).
				Execute(context.Background(), domain.IncomingDocument{Bytes: tc.body})

			if !errors.Is(err, tc.want) {
				t.Fatalf("Execute() = %v, quería %v", err, tc.want)
			}
			if extractor.called {
				t.Fatal("llamó al Extractor antes de pasar la puerta 1")
			}
			if len(publisher.events) != 0 {
				t.Fatalf("publicó %d eventos, quería 0", len(publisher.events))
			}
		})
	}
}

func TestExtractUseCasePublishesNothingWhenExtractionFails(t *testing.T) {
	t.Run("el Extractor no responde", func(t *testing.T) {
		publisher := &publisherSpy{}

		_, err := NewExtractUseCase(&extractorStub{err: domain.ErrExtractorUnavailable}, &summarizerSpy{}, publisher).
			Execute(context.Background(), domain.IncomingDocument{Bytes: []byte(validPDFBytes)})

		if !errors.Is(err, domain.ErrExtractorUnavailable) {
			t.Fatalf("Execute() = %v, quería ErrExtractorUnavailable", err)
		}
		if len(publisher.events) != 0 {
			t.Fatalf("publicó %d eventos, quería 0", len(publisher.events))
		}
	})

	t.Run("la puerta 2 rechaza el motivo", func(t *testing.T) {
		extractor := &extractorStub{result: domain.Extraction{Reason: domain.ReasonEncryptedPDF}}
		publisher := &publisherSpy{}

		_, err := NewExtractUseCase(extractor, &summarizerSpy{}, publisher).
			Execute(context.Background(), domain.IncomingDocument{Bytes: []byte(validPDFBytes)})

		var domainErr *domain.Error
		if !errors.As(err, &domainErr) || domainErr.Code != "extraction_failed" || domainErr.Reason != domain.ReasonEncryptedPDF {
			t.Fatalf("Execute() = %v, quería extraction_failed/encrypted_pdf", err)
		}
		if len(publisher.events) != 0 {
			t.Fatalf("publicó %d eventos, quería 0", len(publisher.events))
		}
	})
}

func TestExtractUseCasePublishesNothingWhenSummaryFails(t *testing.T) {
	extractor := &extractorStub{result: domain.Extraction{Text: "texto plano", Reason: domain.ReasonSuccess}}
	publisher := &publisherSpy{}

	_, err := NewExtractUseCase(extractor, &summarizerSpy{err: domain.ErrSummaryFailed}, publisher).
		Execute(context.Background(), domain.IncomingDocument{
			Bytes:     []byte(validPDFBytes),
			Summarize: true,
		})

	if !errors.Is(err, domain.ErrSummaryFailed) {
		t.Fatalf("Execute() = %v, quería ErrSummaryFailed", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("publicó %d eventos, quería 0: un 502 no debe dejar nada ingerido", len(publisher.events))
	}
}

func TestExtractUseCasePublishesNothingWhenSummaryIsEmpty(t *testing.T) {
	cases := map[string]string{
		"vacío":         "",
		"solo espacios": "   ",
		"solo saltos":   "\n\t\n",
	}

	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			extractor := &extractorStub{result: domain.Extraction{Text: "texto plano", Reason: domain.ReasonSuccess}}
			summarizer := &summarizerSpy{result: domain.Summary{Text: text}}
			publisher := &publisherSpy{}

			_, err := NewExtractUseCase(extractor, summarizer, publisher).
				Execute(context.Background(), domain.IncomingDocument{
					Bytes:     []byte(validPDFBytes),
					Summarize: true,
				})

			if !errors.Is(err, domain.ErrSummaryFailed) {
				t.Fatalf("Execute() = %v, quería ErrSummaryFailed", err)
			}
			if len(publisher.events) != 0 {
				t.Fatalf("publicó %d eventos, quería 0: un resumen vacío no se publica", len(publisher.events))
			}
		})
	}
}
