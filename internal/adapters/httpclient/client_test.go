package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestNewConfiguraTimeoutYPool(t *testing.T) {
	client := New(3 * time.Second)
	if client.Timeout != 3*time.Second {
		t.Errorf("timeout = %s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.MaxIdleConnsPerHost != maxIdleConnsPerHost {
		t.Fatalf("transporte inesperado: %#v", client.Transport)
	}
	if transport == http.DefaultTransport {
		t.Error("no debe reusar el transporte global")
	}
}
