package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Semaphore acota cuántas peticiones corren a la vez. La extracción usa uno ancho y el resumen uno
// angosto, porque son el mismo Resumidor el que serializa lento. El cupo es por réplica, no global
// (ADR-0007).
type Semaphore struct {
	slots chan struct{}
}

// NewSemaphore crea un semáforo con `capacity` cupos. Una capacidad no positiva lo deja sin límite.
func NewSemaphore(capacity int) *Semaphore {
	if capacity <= 0 {
		return &Semaphore{}
	}
	return &Semaphore{slots: make(chan struct{}, capacity)}
}

// tryAcquire toma un cupo sin bloquear; devuelve si lo consiguió.
func (s *Semaphore) tryAcquire() bool {
	if s.slots == nil {
		return true
	}
	select {
	case s.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Semaphore) release() {
	if s.slots == nil {
		return
	}
	select {
	case <-s.slots:
	default:
	}
}

// Available es cuántos cupos quedan libres. Pensado para diagnóstico y tests.
func (s *Semaphore) Available() int {
	if s.slots == nil {
		return 0
	}
	return cap(s.slots) - len(s.slots)
}

// Concurrency rechaza con `429` y `Retry-After` cuando el semáforo está lleno, en vez de encolar la
// petición: el cliente sabe de inmediato que debe reintentar. El cupo se libera siempre, incluso si
// el handler entra en panic.
func Concurrency(sem *Semaphore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sem.tryAcquire() {
			c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    "concurrency_limited",
				"message": "tope de concurrencia alcanzado en esta réplica",
			})
			return
		}
		defer sem.release()
		c.Next()
	}
}

// retryAfterSeconds es la espera sugerida cuando el semáforo está lleno. No hay una duración real
// que estimar: el trabajo en curso es de duración variable, así que se sugiere un segundo, que es
// suficiente para que un cliente bien portado reintente sin martillar.
const retryAfterSeconds = 1
