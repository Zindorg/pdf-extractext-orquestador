package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCorrelationSetsDocumentIDHeaderFromPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Correlation())
	router.GET("/api/v1/documents/:document_id", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+documentID, nil))

	if got := rec.Header().Get("X-Document-Id"); got != documentID {
		t.Fatalf("X-Document-Id = %q, quería %q", got, documentID)
	}
}

func TestCorrelationOmitsHeaderWithoutDocumentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Correlation())
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := rec.Header().Get("X-Document-Id"); got != "" {
		t.Fatalf("X-Document-Id = %q, quería vacío en una ruta sin documento", got)
	}
}
