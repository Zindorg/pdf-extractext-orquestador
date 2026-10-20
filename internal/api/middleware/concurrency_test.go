package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConcurrencyAllowsUpToCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sem := NewSemaphore(2)
	router := gin.New()
	router.Use(Concurrency(sem))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200", rec.Code)
	}
}

func TestConcurrencyRejectsWhenSaturated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sem := NewSemaphore(1)

	release := make(chan struct{})
	entered := make(chan struct{})
	router := gin.New()
	router.Use(Concurrency(sem))
	router.GET("/slow", func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusOK)
	})

	go func() {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()
	<-entered

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quería 429 con el semáforo lleno", rec.Code)
	}
	if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("falta Retry-After")
	} else if seconds, err := strconv.Atoi(retryAfter); err != nil || seconds <= 0 {
		t.Fatalf("Retry-After = %q, quería segundos positivos", retryAfter)
	}

	close(release)
}

func TestConcurrencyReleasesSlotAfterRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sem := NewSemaphore(1)
	router := gin.New()
	router.Use(Concurrency(sem))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("petición %d = %d, quería 200: el cupo debe liberarse", i+1, rec.Code)
		}
	}
	if free := sem.Available(); free != 1 {
		t.Fatalf("cupo libre = %d, quería 1 al terminar", free)
	}
}

func TestConcurrencyReleasesSlotOnPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sem := NewSemaphore(1)
	router := gin.New()
	router.Use(Recovery(), Concurrency(sem))
	router.GET("/boom", func(*gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if free := sem.Available(); free != 1 {
		t.Fatalf("cupo libre = %d tras un panic, quería 1: no debe quedar tomado", free)
	}
}
