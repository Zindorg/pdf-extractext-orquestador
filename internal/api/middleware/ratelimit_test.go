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

func TestRateLimitUsaElClienteDetrasDeUnProxyConfiable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// httptest usa RemoteAddr 192.0.2.1: es el proxy (Traefik).
	if err := router.SetTrustedProxies([]string{"192.0.2.0/24", "10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	router.Use(RateLimit(RateLimitConfig{Limit: 1, Window: time.Minute}))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func(xff string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		req.Header.Set("X-Forwarded-For", xff)
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	do("203.0.113.7, 10.0.0.1")
	if code := do("203.0.113.7, 10.0.0.99"); code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quería 429: el cliente es 203.0.113.7, no el proxy", code)
	}
	if code := do("198.51.100.9"); code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: otro cliente no comparte el límite", code)
	}
}

func TestRateLimitIgnoraXForwardedForDeUnClienteDirecto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	router.Use(RateLimit(RateLimitConfig{Limit: 1, Window: time.Minute}))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i, xff := range []string{"203.0.113.1", "203.0.113.2"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		req.Header.Set("X-Forwarded-For", xff)
		router.ServeHTTP(rec, req)
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d: cambiar X-Forwarded-For no debe esquivar el límite", rec.Code)
		}
	}
}

func TestRateLimitBarreLasVentanasVencidas(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRateLimiter(RateLimitConfig{Limit: 1, Window: time.Minute})
	limiter.now = func() time.Time { return now }

	for i := 0; i < 100; i++ {
		limiter.allow("203.0.113." + strconv.Itoa(i))
	}
	now = now.Add(2 * time.Minute)
	limiter.allow("198.51.100.1")

	if got := limiter.size(); got != 1 {
		t.Fatalf("clientes en memoria = %d, quería 1: las ventanas vencidas deben borrarse", got)
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
