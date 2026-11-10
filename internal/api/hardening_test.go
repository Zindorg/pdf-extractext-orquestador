package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestExtractRechazaUnCuerpoMayorAlTopeSinLlegarAlCasoDeUso(t *testing.T) {
	extract := &extractUseCaseStub{}
	router := mustNewRouter(Deps{
		Extract:          extract,
		Summary:          &summaryUseCaseStub{},
		Proxy:            &proxyUseCaseStub{},
		PublicBaseURL:    "https://api.example.com",
		MaxDocumentBytes: 1 << 20,
	})
	body, contentType := multipartBody(t, nil, append([]byte("%PDF-"), make([]byte, 5<<20)...))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, quería 413; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if extract.called {
		t.Fatal("un documento por encima del tope no debe llegar al caso de uso")
	}
}

func TestExtractRechazaUnArchivoApenasMayorAlTope(t *testing.T) {
	extract := &extractUseCaseStub{}
	router := mustNewRouter(Deps{
		Extract:          extract,
		Summary:          &summaryUseCaseStub{},
		Proxy:            &proxyUseCaseStub{},
		MaxDocumentBytes: 1 << 20,
	})
	// Entra en el margen del multipart pero el archivo supera el tope por un byte.
	body, contentType := multipartBody(t, nil, append([]byte("%PDF-"), make([]byte, (1<<20)-4)...))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge || extract.called {
		t.Fatalf("status = %d, called = %v; quería 413 sin llamar al caso de uso", rec.Code, extract.called)
	}
}

func TestUnErrorInternoQuedaEnElLogConSuCausa(t *testing.T) {
	var logs bytes.Buffer
	router := mustNewRouter(Deps{
		Extract: &extractUseCaseStub{},
		Summary: &summaryUseCaseStub{},
		Proxy:   &proxyUseCaseStub{err: errors.New("llamar al Persistidor: connection refused")},
		Logger:  slog.New(slog.NewJSONHandler(&logs, nil)),
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+testDocumentID, nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatal("el cuerpo no debe exponer la causa interna")
	}
	for _, want := range []string{`"level":"ERROR"`, "connection refused", testDocumentID} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log = %s; falta %q", logs.String(), want)
		}
	}
}

func TestReadyNoExponeElMotivo(t *testing.T) {
	var logs bytes.Buffer
	router := mustNewRouter(Deps{
		Extract: &extractUseCaseStub{},
		Summary: &summaryUseCaseStub{},
		Proxy:   &proxyUseCaseStub{},
		Logger:  slog.New(slog.NewJSONHandler(&logs, nil)),
		Ready:   func(context.Context) error { return fmt.Errorf("dial tcp redis:6379: connection refused") },
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "redis") {
		t.Fatalf("el cuerpo expone el motivo interno: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "redis:6379") {
		t.Fatalf("el motivo debe quedar en el log: %s", logs.String())
	}
}

func TestUnPanicResponde500YQuedaEnElLogDeAcceso(t *testing.T) {
	var logs bytes.Buffer
	router := mustNewRouter(Deps{
		Extract: &extractUseCaseStub{},
		Summary: &summaryUseCaseStub{},
		Proxy:   &proxyUseCaseStub{},
		Logger:  slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	router.GET("/boom", func(*gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), `"status":500`) {
		t.Fatalf("status = %d, log = %s", rec.Code, logs.String())
	}
}

// mustNewRouter arma el router de prueba y corta el test si la configuración es inválida.
func mustNewRouter(deps Deps) *gin.Engine {
	router, err := NewRouter(deps)
	if err != nil {
		panic(err)
	}
	return router
}

func TestNewRouterRechazaProxiesInvalidos(t *testing.T) {
	if _, err := NewRouter(Deps{TrustedProxies: []string{"no-es-una-red"}}); err == nil {
		t.Fatal("se esperaba error con TRUSTED_PROXIES inválida")
	}
}

func TestPersistidorCaidoResponde503ConSuCodigo(t *testing.T) {
	router := mustNewRouter(Deps{
		Extract: &extractUseCaseStub{},
		Summary: &summaryUseCaseStub{},
		Proxy:   &proxyUseCaseStub{err: fmt.Errorf("%w: llamar al Persistidor: connection refused", domain.ErrPersisterUnavailable)},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+testDocumentID, nil))

	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"persister_unavailable"`) {
		t.Fatalf("status = %d, cuerpo = %s; quería 503 persister_unavailable", rec.Code, rec.Body.String())
	}
}
