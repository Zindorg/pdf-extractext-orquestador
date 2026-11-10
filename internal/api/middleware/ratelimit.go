package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitConfig parametriza el límite por cliente. `Limit` es cuántas peticiones se admiten por
// `Window`; el default de `Limit` (0) desactiva el límite.
type RateLimitConfig struct {
	Limit  int
	Window time.Duration
}

// rateLimiter cuenta peticiones por cliente en una ventana fija. El estado vive en memoria del
// proceso, así que el límite es por réplica: un deployment con varias réplicas admite
// proporcionalmente más (ADR-0007). Las ventanas vencidas se barren una vez por ventana, para que
// el mapa no crezca con cada IP que pasó alguna vez.
type rateLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	now       func() time.Time
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	return &rateLimiter{
		limit:   cfg.Limit,
		window:  cfg.Window,
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// allow consume un cupo del cliente y devuelve si la petición pasa y cuánto falta para que la
// ventana se reinicie.
func (l *rateLimiter) allow(client string) (bool, time.Duration) {
	if l.limit <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	current, ok := l.buckets[client]
	if !ok || !now.Before(current.resetAt) {
		l.buckets[client] = &bucket{count: 1, resetAt: now.Add(l.window)}
		return true, 0
	}
	if current.count >= l.limit {
		return false, current.resetAt.Sub(now)
	}
	current.count++
	return true, 0
}

// sweep borra las ventanas ya vencidas. Corre a lo sumo una vez por ventana: el costo es lineal en
// los clientes activos y se amortiza entre todas las peticiones de esa ventana.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	for client, b := range l.buckets {
		if !now.Before(b.resetAt) {
			delete(l.buckets, client)
		}
	}
	l.lastSweep = now
}

// size es la cantidad de clientes con ventana abierta. Pensado para tests.
func (l *rateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// RateLimit limita por IP de cliente. La IP sale de c.ClientIP(): gin solo considera
// X-Forwarded-For cuando la conexión viene de un proxy confiable (engine.SetTrustedProxies, que se
// configura con TRUSTED_PROXIES). Así un cliente que llega directo no puede elegir su IP mandando el
// header.
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	limiter := newRateLimiter(cfg)
	return func(c *gin.Context) {
		allowed, retryAfter := limiter.allow(c.ClientIP())
		if !allowed {
			seconds := int(retryAfter.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    "rate_limited",
				"message": "límite de peticiones alcanzado",
			})
			return
		}
		c.Next()
	}
}
