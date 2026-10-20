package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const documentID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestAccessLogRecordsMethodPathAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	router := gin.New()
	router.Use(Correlation(), AccessLog(logger))
	router.GET("/api/v1/documents/:document_id", func(c *gin.Context) { c.Status(http.StatusNotFound) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+documentID, nil))

	logged := buf.String()
	for _, want := range []string{`"method":"GET"`, "/api/v1/documents/", `"status":404`} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log = %q, falta %q", logged, want)
		}
	}
}

func TestAccessLogCarriesDocumentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	router := gin.New()
	router.Use(Correlation(), AccessLog(logger))
	router.GET("/api/v1/documents/:document_id", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+documentID, nil))

	if !strings.Contains(buf.String(), `"document_id":"`+documentID+`"`) {
		t.Fatalf("log = %q, no lleva el document_id como correlador", buf.String())
	}
}

func TestAccessLogLogsRequestsWithoutDocumentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	router := gin.New()
	router.Use(AccessLog(logger))
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	logged := buf.String()
	if !strings.Contains(logged, `"path":"/health"`) {
		t.Fatalf("log = %q, no registró la ruta sin document_id", logged)
	}
	if strings.Contains(logged, "document_id") {
		t.Fatalf("log = %q, no debe inventar un document_id", logged)
	}
}
