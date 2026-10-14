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
	called bool
	result domain.Extraction
	err    error
}

func (s *extractorStub) Extract(context.Context, domain.IncomingDocument) (domain.Extraction, error) {
	s.called = true
	return s.result, s.err
}

type publisherSpy struct {
	events []domain.Event
}

func (s *publisherSpy) Publish(_ context.Context, event domain.Event) error {
	s.events = append(s.events, event)
	return nil
}

func TestExtractUseCasePublishesOriginalWithoutSummary(t *testing.T) {
	extractor := &extractorStub{result: domain.Extraction{
		Text:   "texto plano",
		TimeMS: 1240,
		Reason: domain.ReasonSuccess,
	}}
	publisher := &publisherSpy{}

	result, err := NewExtractUseCase(extractor, publisher).Execute(context.Background(), domain.IncomingDocument{
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

			_, err := NewExtractUseCase(extractor, publisher).Execute(context.Background(), domain.IncomingDocument{Bytes: tc.body})

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

		_, err := NewExtractUseCase(&extractorStub{err: domain.ErrExtractorUnavailable}, publisher).
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

		_, err := NewExtractUseCase(extractor, publisher).
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
