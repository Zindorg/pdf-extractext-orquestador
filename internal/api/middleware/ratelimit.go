package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
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
// proporcionalmente más (ADR-0007).
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	buckets map[string]*bucket
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
	current, ok := l.buckets[client]
	if !ok || !now.Before(current.resetAt) {
		l.buckets[client] = &bucket{count: 1, resetAt: now.Add(l.window)}
		return true, 0
	}
	if current.count >= l.limit {
		return false, time.Until(current.resetAt)
	}
	current.count++
	return true, 0
}

// RateLimit limita por IP. Confía solo en el primer `X-Forwarded-For`: es la IP del cliente detrás
// de Traefik, y confiar en toda la lista dejaría que un cliente mintiera sobre su origen.
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	limiter := newRateLimiter(cfg)
	return func(c *gin.Context) {
		client := clientIP(c)
		allowed, retryAfter := limiter.allow(client)
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

// clientIP devuelve la IP del cliente: el primer `X-Forwarded-For` si existe, y si no la IP del
// par. Traefik agrega el cliente real a la izquierda de la cadena.
func clientIP(c *gin.Context) string {
	if forwarded := c.GetHeader("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
