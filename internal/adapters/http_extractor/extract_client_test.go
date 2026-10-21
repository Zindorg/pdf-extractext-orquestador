package http_extractor

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// testClient arma un cliente de prueba con espera de reintento en cero, para que la suite no tome tiempo.
func testClient(baseURL string) *Client {
	client := New(baseURL, time.Second)
	client.backoff = func(int) time.Duration { return 0 }
	return client
}

func testDocument() domain.IncomingDocument {
	return domain.IncomingDocument{
		Bytes:    []byte("%PDF-1.7\nhola"),
		Filename: "contrato.pdf",
	}
}

func TestExtractSendsContractShape(t *testing.T) {
	var fields map[string]string
	var documentFilename string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método = %s, quería POST", r.Method)
		}
		if r.URL.Path != "/extract" {
			t.Errorf("path = %s, quería /extract", r.URL.Path)
		}

		file, header, err := r.FormFile("document")
		if err != nil {
			t.Errorf("FormFile(document) falló: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		documentFilename = header.Filename

		for _, name := range []string{"document_id", "mime_type"} {
			fields[name] = r.FormValue(name)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"extracted_text":     "texto plano",
			"extraction_time_ms": 1240,
		})
	}))
	defer server.Close()

	fields = make(map[string]string)
	documentID := domain.DocumentID("0f8fad5b-d9cb-469f-a165-70867728950e")

	if _, err := testClient(server.URL).Extract(t.Context(), documentID, testDocument()); err != nil {
		t.Fatalf("Extract() falló: %v", err)
	}

	if fields["document_id"] != string(documentID) {
		t.Errorf("document_id = %q, quería %q", fields["document_id"], documentID)
	}
	if fields["mime_type"] != "application/pdf" {
		t.Errorf("mime_type = %q, quería application/pdf", fields["mime_type"])
	}
	if documentFilename != "contrato.pdf" {
		t.Errorf("filename = %q, quería contrato.pdf", documentFilename)
	}
}

func TestExtractMapsSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"extracted_text":     "texto plano",
			"extraction_time_ms": 1240,
			"warnings": []map[string]string{
				{"code": "partial", "message": "texto extraído parcialmente"},
			},
		})
	}))
	defer server.Close()

	extraction, err := testClient(server.URL).Extract(t.Context(), "id", testDocument())
	if err != nil {
		t.Fatalf("Extract() falló: %v", err)
	}

	if extraction.Text != "texto plano" {
		t.Errorf("Text = %q, quería texto plano", extraction.Text)
	}
	if extraction.TimeMS != 1240 {
		t.Errorf("TimeMS = %d, quería 1240", extraction.TimeMS)
	}
	if extraction.Reason != domain.ReasonSuccess {
		t.Errorf("Reason = %q, quería success", extraction.Reason)
	}
	if len(extraction.Warnings) != 1 || extraction.Warnings[0].Code != "partial" {
		t.Errorf("Warnings = %+v, quería una warning 'partial'", extraction.Warnings)
	}
}

func TestExtractMapsRejectionToReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"reason":  "encrypted_pdf",
			"message": "el PDF está cifrado",
		})
	}))
	defer server.Close()

	extraction, err := testClient(server.URL).Extract(t.Context(), "id", testDocument())
	if err != nil {
		t.Fatalf("Extract() con 422 no debe fallar: %v", err)
	}

	if extraction.Reason != domain.ReasonEncryptedPDF {
		t.Errorf("Reason = %q, quería encrypted_pdf", extraction.Reason)
	}
	if extraction.Text != "" {
		t.Errorf("Text = %q, quería vacío en un rechazo", extraction.Text)
	}
}

func TestExtractTreatsUnreadableRejectionAsWorkerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte("{no es json"))
	}))
	defer server.Close()

	extraction, err := testClient(server.URL).Extract(t.Context(), "id", testDocument())
	if err != nil {
		t.Fatalf("Extract() con cuerpo ilegible no debe fallar en el adaptador: %v", err)
	}

	// El motivo vacío lo clasifica la puerta 2 como fallo del Extractor; acá no se inventa.
	if extraction.Reason != "" {
		t.Errorf("Reason = %q, quería vacío", extraction.Reason)
	}
}

func TestExtractUnavailableOnUnexpectedStatus(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			_, err := testClient(server.URL).Extract(t.Context(), "id", testDocument())
			if !errors.Is(err, domain.ErrExtractorUnavailable) {
				t.Fatalf("Extract() = %v, quería ErrExtractorUnavailable", err)
			}
		})
	}
}

// failingTransport falla a nivel de red y cuenta los intentos, para probar la política de reintentos.
type failingTransport struct{ calls int }

func (t *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, errors.New("conexión rechazada")
}

func TestExtractRetriesNetworkErrors(t *testing.T) {
	transport := &failingTransport{}
	client := testClient("http://extractor.internal")
	client.httpClient = &http.Client{Transport: transport}

	_, err := client.Extract(t.Context(), "id", testDocument())
	if !errors.Is(err, domain.ErrExtractorUnavailable) {
		t.Fatalf("Extract() = %v, quería ErrExtractorUnavailable", err)
	}

	if want := maxRetries + 1; transport.calls != want {
		t.Errorf("intentos = %d, quería %d (1 inicial + %d reintentos)", transport.calls, want, maxRetries)
	}
}

func TestExtractDoesNotRetryOnRejection(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"reason": "corrupt", "message": "dañado"})
	}))
	defer server.Close()

	if _, err := testClient(server.URL).Extract(t.Context(), "id", testDocument()); err != nil {
		t.Fatalf("Extract() falló: %v", err)
	}

	if hits != 1 {
		t.Errorf("hits = %d, quería 1: un error de validación no se reintenta", hits)
	}
}

func TestExtractMapsEmptyTextToReasonEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"extracted_text":     "",
			"extraction_time_ms": 1,
		})
	}))
	defer server.Close()

	extraction, err := testClient(server.URL).Extract(t.Context(), "id", testDocument())
	if err != nil {
		t.Fatalf("Extract() con texto vacío no debe fallar: %v", err)
	}
	if extraction.Reason != domain.ReasonEmpty {
		t.Errorf("Reason = %q, quería %q", extraction.Reason, domain.ReasonEmpty)
	}
	if extraction.Text != "" {
		t.Errorf("Text = %q, quería vacío", extraction.Text)
	}
}
