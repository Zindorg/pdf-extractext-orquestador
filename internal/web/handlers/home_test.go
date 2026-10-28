package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/render"
)

func TestHomeRendersForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rend, err := render.New()
	if err != nil {
		t.Fatal(err)
	}
	h := New(Deps{}, rend)
	r.GET("/", h.Home)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Generar resumen")) {
		t.Fatalf("missing form")
	}
}
