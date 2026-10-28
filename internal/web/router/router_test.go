package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
)

func TestRouterHasHealthzAndSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	extract := application.NewExtractUseCase(nil, nil, nil, nil)
	summary := application.NewSummaryUseCase(nil, nil, nil)
	proxy := application.NewProxyUseCase(nil)
	if err := Register(r, Deps{Extract: extract, Summary: summary, Proxy: proxy}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") == "" {
		t.Fatalf("missing headers")
	}
}

func TestRouterCsrfBlocksPostWithoutOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	extract := application.NewExtractUseCase(nil, nil, nil, nil)
	summary := application.NewSummaryUseCase(nil, nil, nil)
	proxy := application.NewProxyUseCase(nil)
	if err := Register(r, Deps{Extract: extract, Summary: summary, Proxy: proxy}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
}
