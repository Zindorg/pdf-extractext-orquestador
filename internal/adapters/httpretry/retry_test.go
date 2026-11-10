package httpretry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestRetriable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"sin error", nil, false},
		{"conexión rechazada", &url.Error{Op: "Post", Err: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}}, true},
		{"dial fallido", &net.OpError{Op: "dial", Err: errors.New("no route to host")}, true},
		{"timeout del cliente", &url.Error{Op: "Post", Err: timeoutErr{}}, false},
		{"deadline del contexto", fmt.Errorf("llamar: %w", context.DeadlineExceeded), false},
		{"cancelado", context.Canceled, false},
		{"corte a mitad de respuesta", &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}, false},
		{"host inexistente", &net.DNSError{Err: "no such host", IsNotFound: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Retriable(tc.err); got != tc.want {
				t.Fatalf("Retriable(%v) = %v, quería %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSleepCortaConElContexto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sleep(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep = %v", err)
	}
}

func TestLinearBackoff(t *testing.T) {
	if LinearBackoff(1) != 100*time.Millisecond || LinearBackoff(2) != 200*time.Millisecond {
		t.Fatalf("backoff = %s, %s", LinearBackoff(1), LinearBackoff(2))
	}
}

func TestSleepEsperaOCortaAMitad(t *testing.T) {
	if err := Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("Sleep = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := Sleep(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Sleep = %v, quería cortar con el contexto", err)
	}
}
