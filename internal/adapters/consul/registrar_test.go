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

func TestRegisterWithRetryReintentaHastaLograrlo(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var errs int
	if err := New(srv.URL, "", Registration{ID: "x"}).RegisterWithRetry(ctx, func(error) { errs++ }); err != nil {
		t.Fatalf("RegisterWithRetry: %v", err)
	}
	if calls.Load() != 2 || errs != 1 {
		t.Errorf("llamadas = %d, errores = %d", calls.Load(), errs)
	}
}

func TestRegisterWithRetryCortaConElContexto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New(srv.URL, "", Registration{ID: "x"}).RegisterWithRetry(ctx, nil); err == nil {
		t.Fatal("se esperaba el error del contexto")
	}
}
