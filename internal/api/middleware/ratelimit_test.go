package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimitAllowsUpToTheLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(RateLimitConfig{Limit: 2, Window: time.Minute}))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("petición %d = %d, quería 200", i+1, rec.Code)
		}
	}
}

func TestRateLimitBlocksOverTheLimitWithRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(RateLimitConfig{Limit: 1, Window: time.Minute}))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/ok", nil))

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quería 429", second.Code)
	}
	retryAfter := second.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("falta Retry-After")
	}
	if seconds, err := strconv.Atoi(retryAfter); err != nil || seconds <= 0 {
		t.Fatalf("Retry-After = %q, quería segundos positivos", retryAfter)
	}
}

func TestRateLimitUsesFirstForwardedFor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(RateLimitConfig{Limit: 1, Window: time.Minute}))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	router.ServeHTTP(first, req)

	second := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req2.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.99")
	router.ServeHTTP(second, req2)

	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quería 429: el cliente es el primer X-Forwarded-For, no el proxy", second.Code)
	}

	other := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req3.Header.Set("X-Forwarded-For", "198.51.100.9")
	router.ServeHTTP(other, req3)

	if other.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: otro cliente no comparte el límite", other.Code)
	}
}

func TestRateLimitIsSafeUnderConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(RateLimitConfig{Limit: 5, Window: time.Minute}))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	var mu sync.Mutex
	allowed := 0
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
			if rec.Code == http.StatusOK {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 5 {
		t.Fatalf("permitió %d, quería exactamente 5", allowed)
	}
}
