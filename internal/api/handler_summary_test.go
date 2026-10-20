package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestSummaryHandlerReturns202(t *testing.T) {
	summary := &summaryUseCaseStub{result: domain.SummaryRequestResult{
		DocumentID:       testDocumentID,
		Summary:          "resumen",
		SummaryTimeMS:    8300,
		ProcessingTimeMS: 9100,
	}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+testDocumentID+"/summary", nil)
	rec := httptest.NewRecorder()

	newTestRouter(&extractUseCaseStub{}, summary, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, quería 202; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Document-Id"); got != testDocumentID {
		t.Fatalf("X-Document-Id = %q, quería %q", got, testDocumentID)
	}
	if got := rec.Header().Get("Location"); got != "https://api.example.com/api/v1/documents/"+testDocumentID {
		t.Fatalf("Location = %q, quería la URI absoluta de lectura", got)
	}
	if summary.documentID != domain.DocumentID(testDocumentID) {
		t.Fatalf("document_id = %q, quería %q", summary.documentID, testDocumentID)
	}

	var payload map[string]any
	_ = decodeJSON(rec.Body.Bytes(), &payload)
	if payload["summary_status"] != "RESOLVED" {
		t.Fatalf("summary_status = %v, quería RESOLVED", payload["summary_status"])
	}
	if payload["summary"] != "resumen" {
		t.Fatalf("summary = %v, quería el resumen recién calculado", payload["summary"])
	}
	if payload["processing_time_ms"] != float64(9100) {
		t.Fatalf("processing_time_ms = %v, quería el total de la llamada", payload["processing_time_ms"])
	}
}

func TestSummaryHandlerMapsConflict(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrSummaryAlreadyResolved, http.StatusConflict, "summary_already_resolved"},
		{domain.ErrSummaryPending, http.StatusConflict, "summary_pending"},
		{domain.ErrDocumentNotFound, http.StatusNotFound, "document_not_found"},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			summary := &summaryUseCaseStub{err: tc.err}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+testDocumentID+"/summary", nil)
			rec := httptest.NewRecorder()

			newTestRouter(&extractUseCaseStub{}, summary, &proxyUseCaseStub{}).ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, quería %d", rec.Code, tc.status)
			}
			var payload map[string]any
			_ = decodeJSON(rec.Body.Bytes(), &payload)
			if payload["code"] != tc.code {
				t.Fatalf("code = %v, quería %q", payload["code"], tc.code)
			}
		})
	}
}
