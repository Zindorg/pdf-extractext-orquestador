package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

type extractUseCaseStub struct {
	called bool
	in     domain.IncomingDocument
	result domain.IngestResult
	err    error
}

func (s *extractUseCaseStub) Execute(_ context.Context, in domain.IncomingDocument) (domain.IngestResult, error) {
	s.called = true
	s.in = in
	return s.result, s.err
}

type summaryUseCaseStub struct {
	called     bool
	documentID domain.DocumentID
	result     domain.SummaryRequestResult
	err        error
}

func (s *summaryUseCaseStub) Execute(_ context.Context, documentID domain.DocumentID) (domain.SummaryRequestResult, error) {
	s.called = true
	s.documentID = documentID
	return s.result, s.err
}

type proxyUseCaseStub struct {
	called bool
	req    domain.PersisterRequest
	resp   domain.PersisterResponse
	err    error
}

func (s *proxyUseCaseStub) Forward(_ context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error) {
	s.called = true
	s.req = req
	return s.resp, s.err
}

const testDocumentID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func multipartBody(t *testing.T, fields map[string]string, document []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for name, value := range fields {
		_ = writer.WriteField(name, value)
	}
	if document != nil {
		part, err := writer.CreateFormFile("document", "doc.pdf")
		if err != nil {
			t.Fatalf("CreateFormFile() = %v", err)
		}
		_, _ = part.Write(document)
	}
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

func newTestRouter(extract *extractUseCaseStub, summary *summaryUseCaseStub, proxy *proxyUseCaseStub) http.Handler {
	return NewRouter(Deps{
		Extract:       extract,
		Summary:       summary,
		Proxy:         proxy,
		PublicBaseURL: "https://api.example.com",
		Ready:         func(context.Context) error { return nil },
	})
}

func TestExtractHandlerAcceptsAndReturns202WithLocation(t *testing.T) {
	extract := &extractUseCaseStub{result: domain.IngestResult{
		DocumentID:       testDocumentID,
		Filename:         "doc.pdf",
		Checksum:         "abc",
		DeliveryStatus:   domain.DeliveryIngested,
		SummaryStatus:    domain.SummaryNotRequested,
		ExtractionTimeMS: 1240,
	}}
	body, contentType := multipartBody(t, map[string]string{"summarize": "false"}, []byte("%PDF-1.7 contenido"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	newTestRouter(extract, &summaryUseCaseStub{}, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, quería 202; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Document-Id"); got != testDocumentID {
		t.Fatalf("X-Document-Id = %q, quería %q", got, testDocumentID)
	}
	if got := rec.Header().Get("Location"); got != "https://api.example.com/api/v1/documents/"+testDocumentID {
		t.Fatalf("Location = %q, quería la URI absoluta de lectura", got)
	}
	if !extract.called {
		t.Fatal("no llamó al caso de uso")
	}
	if extract.in.Summarize {
		t.Fatal("summarize = true, quería false")
	}

	var payload map[string]any
	if err := decodeJSON(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if payload["document_id"] != testDocumentID || payload["delivery_status"] != "INGESTED" {
		t.Fatalf("cuerpo = %v, no coincide con la admisión", payload)
	}
	if payload["processing_time_ms"] != float64(1240) {
		t.Fatalf("processing_time_ms = %v, quería la suma de las patas", payload["processing_time_ms"])
	}
	if _, present := payload["summary"]; present {
		t.Fatal("summary no debe estar cuando no se pidió")
	}
	if payload["summary_time_ms"] != nil {
		t.Fatalf("summary_time_ms = %v, quería null cuando no se pidió", payload["summary_time_ms"])
	}
}

func TestExtractHandlerPassesSummarizeTrue(t *testing.T) {
	extract := &extractUseCaseStub{result: domain.IngestResult{
		DocumentID:       testDocumentID,
		DeliveryStatus:   domain.DeliveryIngested,
		SummaryStatus:    domain.SummaryResolved,
		SummaryText:      "resumen",
		SummaryTimeMS:    8300,
		ExtractionTimeMS: 1240,
	}}
	body, contentType := multipartBody(t, map[string]string{"summarize": "true"}, []byte("%PDF-1.7 contenido"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	newTestRouter(extract, &summaryUseCaseStub{}, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, quería 202", rec.Code)
	}
	if !extract.in.Summarize {
		t.Fatal("summarize = false, quería true")
	}
	var payload map[string]any
	_ = decodeJSON(rec.Body.Bytes(), &payload)
	if payload["summary"] != "resumen" {
		t.Fatalf("summary = %v, quería el resumen resuelto", payload["summary"])
	}
	if payload["processing_time_ms"] != float64(1240+8300) {
		t.Fatalf("processing_time_ms = %v, quería extracción+resumen", payload["processing_time_ms"])
	}
}

func TestExtractHandlerRejectsNonBooleanSummarize(t *testing.T) {
	body, contentType := multipartBody(t, map[string]string{"summarize": "quizás"}, []byte("%PDF-1.7 contenido"))
	extract := &extractUseCaseStub{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	newTestRouter(extract, &summaryUseCaseStub{}, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quería 400", rec.Code)
	}
	if extract.called {
		t.Fatal("llamó al caso de uso con un campo inválido")
	}
}

func TestExtractHandlerPropagatesDomainError(t *testing.T) {
	extract := &extractUseCaseStub{err: domain.ErrUnsupportedMediaType}
	body, contentType := multipartBody(t, nil, []byte("no es un pdf"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	newTestRouter(extract, &summaryUseCaseStub{}, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, quería 415", rec.Code)
	}
}
