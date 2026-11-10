package consul

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegisterEnviaLaDefinicionYElToken(t *testing.T) {
	var got serviceDefinition
	var token, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		token = r.Header.Get("X-Consul-Token")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
	}))
	defer srv.Close()

	r := New(srv.URL, "secreto", Registration{
		ID: "orq-abc", Name: "orq", Address: "orchestrator", Port: 8080, Version: "1.2.3", CheckPath: "/ready",
	})
	if err := r.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if path != "/v1/agent/service/register" {
		t.Errorf("path = %q", path)
	}
	if token != "secreto" {
		t.Errorf("token = %q", token)
	}
	if got.ID != "orq-abc" || got.Name != "orq" || got.Address != "orchestrator" || got.Port != 8080 {
		t.Errorf("definición inesperada: %+v", got)
	}
	if got.Check.HTTP != "http://orchestrator:8080/ready" {
		t.Errorf("el check debe apuntar al nombre del servicio, no a localhost: %q", got.Check.HTTP)
	}
	if got.Check.DeregisterCriticalServiceAfter == "" {
		t.Error("falta DeregisterCriticalServiceAfter")
	}
	if got.Meta["version"] != "1.2.3" {
		t.Errorf("meta = %v", got.Meta)
	}
}

func TestRegisterSinTokenNoMandaElHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["X-Consul-Token"]; ok {
			t.Error("no debe mandar X-Consul-Token vacío")
		}
	}))
	defer srv.Close()

	if err := New(srv.URL, "", Registration{ID: "x", Name: "x", Address: "x", Port: 1}).Register(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterDevuelveErrorSiConsulRechaza(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Permission denied", http.StatusForbidden)
	}))
	defer srv.Close()

	if err := New(srv.URL, "", Registration{ID: "x"}).Register(context.Background()); err == nil {
		t.Fatal("se esperaba error con 403")
	}
}

func TestDeregisterUsaElID(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("método = %s", r.Method)
		}
		path = r.URL.Path
	}))
	defer srv.Close()

	if err := New(srv.URL, "", Registration{ID: "orq-abc"}).Deregister(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/agent/service/deregister/orq-abc" {
		t.Errorf("path = %q", path)
	}
}

func TestMaintainReintentaYRenuevaElRegistro(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var registered, errs atomic.Int32
	done := make(chan struct{})
	go func() {
		New(srv.URL, "", Registration{ID: "x"}).Maintain(ctx, 20*time.Millisecond,
			func() { registered.Add(1) }, func(error) { errs.Add(1) })
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	if calls.Load() < 4 {
		t.Fatalf("llamadas = %d: debe reintentar y después renovar el registro", calls.Load())
	}
	if errs.Load() != 1 || registered.Load() != 1 {
		t.Errorf("errores = %d, registros recuperados = %d; quería 1 y 1", errs.Load(), registered.Load())
	}
}

func TestMaintainTerminaConElContexto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		New(srv.URL, "", Registration{ID: "x"}).Maintain(ctx, time.Hour, nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Maintain no terminó al cancelar el contexto")
	}
}
