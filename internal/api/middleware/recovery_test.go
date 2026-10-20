package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRecoveryTurnsPanicIntoInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/boom", func(*gin.Context) { panic("algo se rompió") })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", rec.Code)
	}
	if body := rec.Body.String(); body == "" || body == "algo se rompió" {
		t.Fatalf("cuerpo = %q, quería el envoltorio de error sin el mensaje del panic", body)
	}
}

func TestRecoveryLetsNormalRequestsThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("respuesta = %d/%q, quería 200/ok", rec.Code, rec.Body.String())
	}
}
