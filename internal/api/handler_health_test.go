package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthIsAlwaysOK(t *testing.T) {
	router := NewRouter(Deps{
		Extract:       &extractUseCaseStub{},
		Summary:       &summaryUseCaseStub{},
		Proxy:         &proxyUseCaseStub{},
		PublicBaseURL: "https://api.example.com",
		Ready:         func(context.Context) error { return errors.New("redis caído") },
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200 aunque una dependencia esté caída", rec.Code)
	}
}

func TestReadyReflectsDependencyState(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"dependencias sanas", nil, http.StatusOK},
		{"redis caído", errors.New("redis no disponible"), http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Deps{
				Extract:       &extractUseCaseStub{},
				Summary:       &summaryUseCaseStub{},
				Proxy:         &proxyUseCaseStub{},
				PublicBaseURL: "https://api.example.com",
				Ready:         func(context.Context) error { return tc.err },
			})

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, quería %d; cuerpo: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
