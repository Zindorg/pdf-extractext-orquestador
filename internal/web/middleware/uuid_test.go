package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateUUIDRejectsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/documents/:id", ValidateUUID("id"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/documents/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestValidateUUIDAcceptsValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/documents/:id", ValidateUUID("id"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/documents/0f8fad5b-d9cb-469f-a165-70867728950e", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}
