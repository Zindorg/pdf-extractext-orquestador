package http_summarizer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

const (
	testDocumentID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testText       = "El contrato tiene doce cláusulas."
)

func TestSummarizeSendsContractShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método = %s, quería POST", r.Method)
		}
		if r.URL.Path != "/summarize" {
			t.Errorf("path = %s, quería /summarize", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type = %q, quería application/json", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("leer el cuerpo falló: %v", err)
		}

		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("cuerpo ilegible: %v (%s)", err, body)
		}
		if payload["document_id"] != testDocumentID {
			t.Errorf("document_id = %q, quería %q", payload["document_id"], testDocumentID)
		}
		if payload["extracted_text"] != testText {
			t.Errorf("extracted_text = %q, quería %q", payload["extracted_text"], testText)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"summary":         "Doce cláusulas.",
			"summary_time_ms": 4200,
		})
	}))
	defer server.Close()

	if _, err := testClient(server.URL).Summarize(t.Context(), domain.DocumentID(testDocumentID), testText); err != nil {
		t.Fatalf("Summarize() falló: %v", err)
	}
}

func TestSummarizeMapsSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"summary":         "Doce cláusulas.",
			"summary_time_ms": 4200,
		})
	}))
	defer server.Close()

	summary, err := testClient(server.URL).Summarize(t.Context(), domain.DocumentID(testDocumentID), testText)
	if err != nil {
		t.Fatalf("Summarize() falló: %v", err)
	}

	if summary.Text != "Doce cláusulas." {
		t.Errorf("Text = %q, quería Doce cláusulas.", summary.Text)
	}
	if summary.TimeMS != 4200 {
		t.Errorf("TimeMS = %d, quería 4200", summary.TimeMS)
	}
}

// Un 422 (document_too_long) es 502 summary_failed en la API: el resumen se pidió y falló, y por la
// regla de publicar solo al final no se ingirió nada (workers.md).
func TestSummarizeMapsRejectionToSummaryFailed(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"reason":  "document_too_long",
			"message": "el texto supera la ventana de contexto",
		})
	}))
	defer server.Close()

	_, err := testClient(server.URL).Summarize(t.Context(), domain.DocumentID(testDocumentID), testText)
	if !errors.Is(err, domain.ErrSummaryFailed) {
		t.Fatalf("Summarize() = %v, quería ErrSummaryFailed", err)
	}
	if hits != 1 {
		t.Errorf("hits = %d, quería 1: un error de validación no se reintenta", hits)
	}
}

// Un motivo ilegible es un fallo del worker: no se inventa una categoría (workers.md).
func TestSummarizeTreatsUnreadableRejectionAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte("{no es json"))
	}))
	defer server.Close()

	_, err := testClient(server.URL).Summarize(t.Context(), domain.DocumentID(testDocumentID), testText)
	if !errors.Is(err, domain.ErrSummaryFailed) {
		t.Fatalf("Summarize() = %v, quería ErrSummaryFailed", err)
	}
}

func TestSummarizeFailedOnUnexpectedStatus(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			_, err := testClient(server.URL).Summarize(t.Context(), domain.DocumentID(testDocumentID), testText)
			if !errors.Is(err, domain.ErrSummaryFailed) {
				t.Fatalf("Summarize() = %v, quería ErrSummaryFailed", err)
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

func TestSummarizeRetriesNetworkErrors(t *testing.T) {
	transport := &failingTransport{}
	client := testClient("http://summarizer.internal")
	client.httpClient = &http.Client{Transport: transport}

	_, err := client.Summarize(t.Context(), domain.DocumentID(testDocumentID), testText)
	if !errors.Is(err, domain.ErrSummaryFailed) {
		t.Fatalf("Summarize() = %v, quería ErrSummaryFailed", err)
	}

	if want := maxRetries + 1; transport.calls != want {
		t.Errorf("intentos = %d, quería %d (1 inicial + %d reintentos)", transport.calls, want, maxRetries)
	}
}
