package http_persister

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// testClient arma un cliente apuntando al servidor de prueba.
func testClient(baseURL string) *Client {
	return New(baseURL, time.Second)
}

func TestDoKeepsPathAndQueryUnrewritten(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("método = %s, quería GET", r.Method)
		}
		if r.URL.Path != "/api/v1/documents/0f8fad5b-d9cb-469f-a165-70867728950e" {
			t.Errorf("path = %s, quería /api/v1/documents/... intacto", r.URL.Path)
		}
		if r.URL.RawQuery != "page=2&page_size=50" {
			t.Errorf("query = %q, quería page=2&page_size=50 intacta", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := domain.PersisterRequest{
		Method:   http.MethodGet,
		Path:     "/api/v1/documents/0f8fad5b-d9cb-469f-a165-70867728950e",
		RawQuery: "page=2&page_size=50",
	}
	if _, err := testClient(server.URL).Do(t.Context(), req); err != nil {
		t.Fatalf("Do() falló: %v", err)
	}
}

func TestDoReturnsSuccessResponseRaw(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="original.pdf"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("texto plano del documento"))
	}))
	defer server.Close()

	resp, err := testClient(server.URL).Do(t.Context(), domain.PersisterRequest{
		Method: http.MethodGet,
		Path:   "/api/v1/documents/id/download/original",
	})
	if err != nil {
		t.Fatalf("Do() falló: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, quería 200", resp.StatusCode)
	}
	if string(resp.Body) != "texto plano del documento" {
		t.Errorf("Body = %q, quería el texto tal cual", resp.Body)
	}
	if got := resp.Header["Content-Disposition"]; len(got) != 1 {
		t.Errorf("Content-Disposition = %v, quería que el proxy la conserve", got)
	}
}

func TestDoTranslatesKnownEnvelopeCode(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"404", http.StatusNotFound, "document_not_found"},
		{"409 summary_pending", http.StatusConflict, "summary_pending"},
		{"400", http.StatusBadRequest, "invalid_request"},
		{"409 checksum", http.StatusConflict, "checksum_conflict"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"code":"` + tc.code + `","message":"detalle interno del Persistidor"}`))
			}))
			defer server.Close()

			_, err := testClient(server.URL).Do(t.Context(), domain.PersisterRequest{
				Method: http.MethodGet,
				Path:   "/api/v1/documents/id",
			})

			var domErr *domain.Error
			if !errors.As(err, &domErr) {
				t.Fatalf("Do() = %v, quería un error de dominio tipado", err)
			}
			if domErr.Code != tc.code {
				t.Errorf("Code = %q, quería %q", domErr.Code, tc.code)
			}
			if domErr.Message == "detalle interno del Persistidor" {
				t.Error("el mensaje del Persistidor no debe exponerse al cliente")
			}
		})
	}
}

// Un estado que el vocabulario resuelve sin ambigüedad se traduce aunque el cuerpo no se pueda leer.
func TestDoFallsBackToStatusWhenEnvelopeIsUnreadable(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "invalid_request"},
		{http.StatusNotFound, "document_not_found"},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("<html>proxy al medio</html>"))
			}))
			defer server.Close()

			_, err := testClient(server.URL).Do(t.Context(), domain.PersisterRequest{
				Method: http.MethodGet,
				Path:   "/api/v1/documents/id",
			})

			var domErr *domain.Error
			if !errors.As(err, &domErr) {
				t.Fatalf("Do() = %v, quería un error de dominio tipado", err)
			}
			if domErr.Code != tc.code {
				t.Errorf("Code = %q, quería %q", domErr.Code, tc.code)
			}
		})
	}
}

// Un 409 sin código reconocible depende de la ruta (download/summary vs restore), así que no se
// inventa una categoría: se devuelve como fallo no previsto y la capa api responde 500.
func TestDoTreatsConflictWithoutKnownCodeAsUnexpected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"algo_que_no_conozco"}`))
	}))
	defer server.Close()

	_, err := testClient(server.URL).Do(t.Context(), domain.PersisterRequest{
		Method: http.MethodPost,
		Path:   "/api/v1/documents/id/restore",
	})

	var domErr *domain.Error
	if errors.As(err, &domErr) {
		t.Fatalf("Do() = %v, no quería error de dominio: el code no es del vocabulario", domErr.Code)
	}
	if err == nil {
		t.Fatal("Do() no falló, quería un error")
	}
}

func TestDoFailsOnTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close()

	_, err := testClient(baseURL).Do(t.Context(), domain.PersisterRequest{
		Method: http.MethodGet,
		Path:   "/api/v1/documents",
	})
	if err == nil {
		t.Fatal("Do() no falló contra un servidor caído")
	}

	var domErr *domain.Error
	if errors.As(err, &domErr) {
		t.Errorf("Do() = %v, un fallo de transporte no es un error del documento", domErr.Code)
	}
}
