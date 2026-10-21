package application

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

const (
	demandText     = "texto extraído del documento"
	demandSummary  = "resumen del documento"
	demandLatency  = 50 * time.Millisecond
	resumeLatency  = 800 * time.Millisecond
	demandPath     = "/api/v1/documents/"
	originalSuffix = "/download/original"
)

func documentPath(id string) string { return demandPath + id }

func originalPath(id string) string { return documentPath(id) + originalSuffix }

func documentBody(id string, status domain.SummaryStatus, text string) []byte {
	return []byte(`{"document_id":"` + id + `","delivery_status":"AVAILABLE","summary_status":"` +
		string(status) + `","extracted_text":` + strconv.Quote(text) + `,"summary":null}`)
}

type fakeClock struct {
	current time.Time
}

func (c *fakeClock) Now() time.Time          { return c.current }
func (c *fakeClock) advance(d time.Duration) { c.current = c.current.Add(d) }

type summaryPersisterStub struct {
	requests []domain.PersisterRequest
	bodies   map[string][]byte
	errs     map[string]error
	clock    *fakeClock
	latency  time.Duration
}

func (s *summaryPersisterStub) Do(_ context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error) {
	s.requests = append(s.requests, req)
	if s.clock != nil && s.latency > 0 {
		s.clock.advance(s.latency)
	}
	if err, ok := s.errs[req.Path]; ok {
		return domain.PersisterResponse{}, err
	}
	if body, ok := s.bodies[req.Path]; ok {
		return domain.PersisterResponse{StatusCode: 200, Body: body}, nil
	}
	return domain.PersisterResponse{}, domain.ErrDocumentNotFound
}

func (s *summaryPersisterStub) FindActiveByChecksum(_ context.Context, _ domain.Checksum) (domain.StoredDocumentSummary, bool, error) {
	return domain.StoredDocumentSummary{}, false, nil
}

type demandSummarizerStub struct {
	called     bool
	documentID domain.DocumentID
	text       string
	result     domain.Summary
	err        error
	clock      *fakeClock
	latency    time.Duration
}

func (s *demandSummarizerStub) Summarize(_ context.Context, documentID domain.DocumentID, text string) (domain.Summary, error) {
	s.called = true
	s.documentID = documentID
	s.text = text
	if s.clock != nil && s.latency > 0 {
		s.clock.advance(s.latency)
	}
	return s.result, s.err
}

// newSummaryUseCase arma el caso de uso con un reloj falso compartido por el Persistidor y el
// Resumidor, de modo que el tiempo total de la llamada se pueda fijar a mano en el test.
func newSummaryUseCase(persister domain.PersisterProxy, summarizer domain.Summarizer, publisher domain.EventPublisher, clock *fakeClock) *SummaryUseCase {
	uc := NewSummaryUseCase(persister, summarizer, publisher)
	uc.now = clock.Now
	return uc
}

func TestSummaryUseCasePublishesSingleEventAndMeasuresWholeCall(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
	persister := &summaryPersisterStub{
		clock:   clock,
		latency: demandLatency,
		bodies: map[string][]byte{
			documentPath(demandDocumentID): documentBody(demandDocumentID, domain.SummaryNotRequested, demandText),
			originalPath(demandDocumentID): []byte(demandText),
		},
	}
	summarizer := &demandSummarizerStub{
		result:  domain.Summary{Text: demandSummary, TimeMS: 8300},
		clock:   clock,
		latency: resumeLatency,
	}
	publisher := &publisherSpy{}

	result, err := newSummaryUseCase(persister, summarizer, publisher, clock).
		Execute(context.Background(), domain.DocumentID(demandDocumentID))
	if err != nil {
		t.Fatalf("Execute() = %v, quería nil", err)
	}

	if result.DocumentID != domain.DocumentID(demandDocumentID) {
		t.Fatalf("DocumentID = %q, quería %q", result.DocumentID, demandDocumentID)
	}
	if result.Summary != demandSummary || result.SummaryTimeMS != 8300 {
		t.Fatalf("resumen = %q/%d, quería %q/8300", result.Summary, result.SummaryTimeMS, demandSummary)
	}
	wantTotal := demandLatency + demandLatency + resumeLatency
	if result.ProcessingTimeMS != wantTotal.Milliseconds() {
		t.Fatalf("ProcessingTimeMS = %d, quería %d: el total es resumen más las lecturas por proxy",
			result.ProcessingTimeMS, wantTotal.Milliseconds())
	}

	if summarizer.documentID != domain.DocumentID(demandDocumentID) || summarizer.text != demandText {
		t.Fatalf("Resumidor recibió document_id=%q text=%q, quería %q/%q",
			summarizer.documentID, summarizer.text, demandDocumentID, demandText)
	}
	if len(persister.requests) != 2 {
		t.Fatalf("llegó al Persistidor %d veces, quería 2 (lectura y descarga)", len(persister.requests))
	}
	if persister.requests[0].Path != documentPath(demandDocumentID) {
		t.Fatalf("primer request = %q, quería la lectura antes de resumir", persister.requests[0].Path)
	}
	if persister.requests[1].Path != originalPath(demandDocumentID) {
		t.Fatalf("segundo request = %q, quería la descarga del texto", persister.requests[1].Path)
	}

	if len(publisher.events) != 1 {
		t.Fatalf("publicó %d eventos, quería 1", len(publisher.events))
	}
	resolved, ok := publisher.events[0].(domain.SummaryResolvedEvent)
	if !ok {
		t.Fatalf("evento = %T, quería domain.SummaryResolvedEvent", publisher.events[0])
	}
	if resolved.DocumentID != domain.DocumentID(demandDocumentID) || resolved.Summary != demandSummary {
		t.Fatalf("summary_resolved = %+v, no coincide con la petición", resolved)
	}
	if resolved.Checksum != domain.NewChecksum(demandText) {
		t.Fatalf("evento.Checksum = %q, quería el checksum del texto extraído %q", resolved.Checksum, domain.NewChecksum(demandText))
	}
	if resolved.SummaryTimeMS != 8300 {
		t.Fatalf("evento.SummaryTimeMS = %d, quería 8300", resolved.SummaryTimeMS)
	}
	if resolved.ResolvedAt.IsZero() {
		t.Fatal("el evento no lleva resolved_at")
	}
}

func TestSummaryUseCaseRejectsWhenSummaryAlreadyExists(t *testing.T) {
	cases := []struct {
		name   string
		status domain.SummaryStatus
		want   error
	}{
		{"ya resuelto", domain.SummaryResolved, domain.ErrSummaryAlreadyResolved},
		{"en curso", domain.SummaryPending, domain.ErrSummaryPending},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
			persister := &summaryPersisterStub{
				clock: clock,
				bodies: map[string][]byte{
					documentPath(demandDocumentID): documentBody(demandDocumentID, tc.status, demandText),
				},
			}
			summarizer := &demandSummarizerStub{result: domain.Summary{Text: demandSummary}}
			publisher := &publisherSpy{}

			_, err := newSummaryUseCase(persister, summarizer, publisher, clock).
				Execute(context.Background(), domain.DocumentID(demandDocumentID))

			if !errors.Is(err, tc.want) {
				t.Fatalf("Execute() = %v, quería %v", err, tc.want)
			}
			if summarizer.called {
				t.Fatal("llamó al Resumidor sobre un documento que ya tenía resumen en camino")
			}
			if len(publisher.events) != 0 {
				t.Fatalf("publicó %d eventos, quería 0", len(publisher.events))
			}
			if len(persister.requests) != 1 {
				t.Fatalf("llegó al Persistidor %d veces, quería 1: la guarda corta antes de descargar", len(persister.requests))
			}
		})
	}
}

func TestSummaryUseCaseRetriesFailedSummary(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
	persister := &summaryPersisterStub{
		clock: clock,
		bodies: map[string][]byte{
			documentPath(demandDocumentID): documentBody(demandDocumentID, domain.SummaryFailed, demandText),
			originalPath(demandDocumentID): []byte(demandText),
		},
	}
	summarizer := &demandSummarizerStub{result: domain.Summary{Text: demandSummary}}
	publisher := &publisherSpy{}

	result, err := newSummaryUseCase(persister, summarizer, publisher, clock).
		Execute(context.Background(), domain.DocumentID(demandDocumentID))
	if err != nil {
		t.Fatalf("Execute() = %v, quería que reintente un resumen fallido", err)
	}
	if result.Summary != demandSummary || len(publisher.events) != 1 {
		t.Fatalf("resultado=%+v eventos=%d, quería el resumen resuelto con 1 evento", result, len(publisher.events))
	}
}

func TestSummaryUseCasePropagatesDocumentNotFound(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
	persister := &summaryPersisterStub{errs: map[string]error{
		documentPath(demandDocumentID): domain.ErrDocumentNotFound,
	}}
	summarizer := &demandSummarizerStub{}
	publisher := &publisherSpy{}

	_, err := newSummaryUseCase(persister, summarizer, publisher, clock).
		Execute(context.Background(), domain.DocumentID(demandDocumentID))

	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Fatalf("Execute() = %v, quería ErrDocumentNotFound", err)
	}
	if summarizer.called {
		t.Fatal("llamó al Resumidor sin documento")
	}
	if len(persister.requests) != 1 {
		t.Fatalf("llegó al Persistidor %d veces, quería 1", len(persister.requests))
	}
	if len(publisher.events) != 0 {
		t.Fatalf("publicó %d eventos, quería 0", len(publisher.events))
	}
}

func TestSummaryUseCaseDoesNotPublishWhenSummarizerFails(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
	persister := &summaryPersisterStub{
		clock: clock,
		bodies: map[string][]byte{
			documentPath(demandDocumentID): documentBody(demandDocumentID, domain.SummaryNotRequested, demandText),
			originalPath(demandDocumentID): []byte(demandText),
		},
	}
	publisher := &publisherSpy{}

	_, err := newSummaryUseCase(persister, &demandSummarizerStub{err: domain.ErrSummaryFailed}, publisher, clock).
		Execute(context.Background(), domain.DocumentID(demandDocumentID))

	if !errors.Is(err, domain.ErrSummaryFailed) {
		t.Fatalf("Execute() = %v, quería ErrSummaryFailed", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("publicó %d eventos, quería 0: un 502 no debe quedar publicado", len(publisher.events))
	}
}

func TestSummaryUseCaseDoesNotPublishWhenSummaryIsEmpty(t *testing.T) {
	cases := map[string]string{
		"vacío":         "",
		"solo espacios": "   ",
	}

	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
			persister := &summaryPersisterStub{
				clock: clock,
				bodies: map[string][]byte{
					documentPath(demandDocumentID): documentBody(demandDocumentID, domain.SummaryNotRequested, demandText),
					originalPath(demandDocumentID): []byte(demandText),
				},
			}
			publisher := &publisherSpy{}

			_, err := newSummaryUseCase(persister, &demandSummarizerStub{result: domain.Summary{Text: text}}, publisher, clock).
				Execute(context.Background(), domain.DocumentID(demandDocumentID))

			if !errors.Is(err, domain.ErrSummaryFailed) {
				t.Fatalf("Execute() = %v, quería ErrSummaryFailed", err)
			}
			if len(publisher.events) != 0 {
				t.Fatalf("publicó %d eventos, quería 0: un resumen vacío no se publica", len(publisher.events))
			}
		})
	}
}

func TestSummaryUseCasePropagatesDownloadFailure(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
	persister := &summaryPersisterStub{
		errs: map[string]error{originalPath(demandDocumentID): domain.ErrDocumentNotFound},
		bodies: map[string][]byte{
			documentPath(demandDocumentID): documentBody(demandDocumentID, domain.SummaryNotRequested, demandText),
		},
	}
	summarizer := &demandSummarizerStub{}
	publisher := &publisherSpy{}

	_, err := newSummaryUseCase(persister, summarizer, publisher, clock).
		Execute(context.Background(), domain.DocumentID(demandDocumentID))

	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Fatalf("Execute() = %v, quería ErrDocumentNotFound", err)
	}
	if summarizer.called {
		t.Fatal("llamó al Resumidor sin texto extraído")
	}
	if len(publisher.events) != 0 {
		t.Fatalf("publicó %d eventos, quería 0", len(publisher.events))
	}
}

func TestSummaryUseCasePropagatesPublishFailure(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1_700_000_000, 0)}
	persister := &summaryPersisterStub{
		clock: clock,
		bodies: map[string][]byte{
			documentPath(demandDocumentID): documentBody(demandDocumentID, domain.SummaryNotRequested, demandText),
			originalPath(demandDocumentID): []byte(demandText),
		},
	}
	publisher := &publisherSpy{err: errors.New("stream caído")}

	_, err := newSummaryUseCase(persister, &demandSummarizerStub{result: domain.Summary{Text: demandSummary}}, publisher, clock).
		Execute(context.Background(), domain.DocumentID(demandDocumentID))

	if err == nil {
		t.Fatal("Execute() = nil, quería el error de publicación")
	}
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		t.Fatalf("Execute() = %v, quería un error sin tipar: nadie declaró un code para esto", err)
	}
}
