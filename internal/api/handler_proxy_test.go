package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestProxyHandlerForwardsRequestUntouched(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/documents"},
		{"GET", "/api/v1/documents/" + testDocumentID},
		{"DELETE", "/api/v1/documents/" + testDocumentID},
		{"GET", "/api/v1/documents/" + testDocumentID + "/download/original"},
		{"GET", "/api/v1/documents/" + testDocumentID + "/download/summary"},
		{"POST", "/api/v1/documents/" + testDocumentID + "/restore"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			proxy := &proxyUseCaseStub{resp: domain.PersisterResponse{StatusCode: 200, Body: []byte("ok")}}

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()

			newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, proxy).ServeHTTP(rec, req)

			if !proxy.called {
				t.Fatal("no llamó al caso de uso del proxy")
			}
			if proxy.req.Method != tc.method || proxy.req.Path != tc.path {
				t.Fatalf("reenvió %s %s, quería %s %s", proxy.req.Method, proxy.req.Path, tc.method, tc.path)
			}
		})
	}
}

func TestProxyHandlerPreservesQuery(t *testing.T) {
	proxy := &proxyUseCaseStub{resp: domain.PersisterResponse{StatusCode: 200, Body: []byte("ok")}}
	query := "page=2&page_size=10&status=RESOLVED"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents?"+query, nil)
	rec := httptest.NewRecorder()

	newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, proxy).ServeHTTP(rec, req)

	if proxy.req.RawQuery != query {
		t.Fatalf("RawQuery = %q, quería %q", proxy.req.RawQuery, query)
	}
}

func TestProxyHandlerReturnsResponseUntranslated(t *testing.T) {
	body := []byte("texto extraído")
	proxy := &proxyUseCaseStub{resp: domain.PersisterResponse{
		StatusCode: 200,
		Header:     map[string][]string{"Content-Disposition": {`attachment; filename="doc.txt"`}},
		Body:       body,
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+testDocumentID+"/download/original", nil)
	rec := httptest.NewRecorder()

	newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, proxy).ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, quería 200", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Fatalf("cuerpo = %q, quería el texto tal cual", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got == "" {
		t.Fatal("se perdió la cabecera Content-Disposition del Persistidor")
	}
}

func TestProxyHandlerMapsTypedError(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrDocumentNotFound, http.StatusNotFound, "document_not_found"},
		{domain.ErrSummaryNotAvailable, http.StatusNotFound, "summary_not_available"},
		{domain.ErrSummaryPending, http.StatusConflict, "summary_pending"},
		{domain.ErrChecksumConflict, http.StatusConflict, "checksum_conflict"},
		{domain.ErrInvalidRequest, http.StatusBadRequest, "invalid_request"},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			proxy := &proxyUseCaseStub{err: tc.err}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+testDocumentID, nil)
			rec := httptest.NewRecorder()

			newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, proxy).ServeHTTP(rec, req)

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

func TestProxyHandlerReturns204WithoutBody(t *testing.T) {
	proxy := &proxyUseCaseStub{resp: domain.PersisterResponse{StatusCode: 204}}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/documents/"+testDocumentID, nil)
	rec := httptest.NewRecorder()

	newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, proxy).ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("status = %d, quería 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("cuerpo = %q, quería vacío", rec.Body.String())
	}
}

func TestProxyHandlerRejectsInvalidDocumentID(t *testing.T) {
	proxy := &proxyUseCaseStub{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/no-es-uuid", nil)
	rec := httptest.NewRecorder()

	newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, proxy).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quería 400", rec.Code)
	}
	if proxy.called {
		t.Fatal("reenvió un document_id que no es UUID")
	}
}

func TestDocumentIDHeaderPresent(t *testing.T) {
	extract := &extractUseCaseStub{result: domain.IngestResult{
		DocumentID:     testDocumentID,
		DeliveryStatus: domain.DeliveryIngested,
		SummaryStatus:  domain.SummaryNotRequested,
	}}
	body, contentType := multipartBody(t, nil, []byte("%PDF-1.7 contenido"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	newTestRouter(extract, &summaryUseCaseStub{}, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Header().Get("X-Document-Id") != testDocumentID {
		t.Fatal("falta X-Document-Id")
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/no-existe", nil)
	rec := httptest.NewRecorder()

	newTestRouter(&extractUseCaseStub{}, &summaryUseCaseStub{}, &proxyUseCaseStub{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quería 404", rec.Code)
	}
}
