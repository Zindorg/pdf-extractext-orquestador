package render

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHTMLRespondeLaPaginaConSuEstado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	r.HTML(c, http.StatusNotFound, "error.html", map[string]any{"Title": "No está", "Message": "nada"})

	if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status = %d, content-type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("las páginas no deben cachearse")
	}
}

func TestHTMLConPlantillaInexistenteRespondeErrorYLoRegistra(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	r.HTML(c, http.StatusOK, "no-existe.html", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", rec.Code)
	}
	if c.Errors.Last() == nil {
		t.Fatal("la causa debe quedar adjunta para el log de acceso")
	}
}

func TestStaticFSSirveLosEstaticos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.StaticFS("/static", StaticFS())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("status = %d, largo = %d", rec.Code, rec.Body.Len())
	}
}
