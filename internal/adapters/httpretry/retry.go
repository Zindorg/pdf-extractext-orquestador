// Package httpretry concentra la política de reintentos de los clientes HTTP hacia los workers.
package httpretry

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"
)

// Retriable dice si vale la pena reintentar una llamada fallida. Solo se reintenta cuando la
// petición no llegó al otro lado: el servicio no aceptó la conexión. Un timeout no se reintenta: el
// worker pudo haber recibido el trabajo y seguir procesándolo, y reenviarlo duplica la carga de un
// servicio que ya está lento (el Resumidor atiende de a uno).
func Retriable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && !dnsErr.IsNotFound
}

// LinearBackoff crece con el intento: 100ms, 200ms, ...
func LinearBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * 100 * time.Millisecond
}

// Sleep espera d o corta con el contexto. Un d menor o igual a cero no espera.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
