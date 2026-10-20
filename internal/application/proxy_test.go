package application

import (
	"context"
	"errors"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

type proxyPersisterStub struct {
	requests []domain.PersisterRequest
	response domain.PersisterResponse
	err      error
}

func (s *proxyPersisterStub) Do(_ context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error) {
	s.requests = append(s.requests, req)
	return s.response, s.err
}

const demandDocumentID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestForwardAllowsOnlyDocumentedRoutes(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/documents"},
		{"GET", "/api/v1/documents/" + demandDocumentID},
		{"DELETE", "/api/v1/documents/" + demandDocumentID},
		{"GET", "/api/v1/documents/" + demandDocumentID + "/download/original"},
		{"GET", "/api/v1/documents/" + demandDocumentID + "/download/summary"},
		{"POST", "/api/v1/documents/" + demandDocumentID + "/restore"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			persister := &proxyPersisterStub{response: domain.PersisterResponse{StatusCode: 200}}

			_, err := NewProxyUseCase(persister).Forward(context.Background(), domain.PersisterRequest{
				Method: tc.method,
				Path:   tc.path,
			})
			if err != nil {
				t.Fatalf("Forward() = %v, quería que pase", err)
			}
			if len(persister.requests) != 1 {
				t.Fatalf("llegó al Persistidor %d veces, quería 1", len(persister.requests))
			}
		})
	}
}

func TestForwardRejectsUndocumentedRoutesBeforeTouchingPersister(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"crear documentos", "POST", "/api/v1/documents"},
		{"borrar el listado", "DELETE", "/api/v1/documents"},
		{"actualizar un documento", "PUT", "/api/v1/documents/" + demandDocumentID},
		{"pedir el resumen por el proxy", "POST", "/api/v1/documents/" + demandDocumentID + "/summary"},
		{"sufijo de descarga desconocido", "GET", "/api/v1/documents/" + demandDocumentID + "/download/pdf"},
		{"sufijo desconocido", "GET", "/api/v1/documents/" + demandDocumentID + "/upload"},
		{"subdirectorio bajo un documento", "GET", "/api/v1/documents/" + demandDocumentID + "/def"},
		{"restaurar el listado", "POST", "/api/v1/documents/restore"},
		{"id vacío", "GET", "/api/v1/documents/"},
		{"fuera del prefijo", "GET", "/documents/" + demandDocumentID},
		{"raíz", "GET", "/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persister := &proxyPersisterStub{response: domain.PersisterResponse{StatusCode: 200}}

			_, err := NewProxyUseCase(persister).Forward(context.Background(), domain.PersisterRequest{
				Method: tc.method,
				Path:   tc.path,
			})
			if !errors.Is(err, domain.ErrInvalidRequest) {
				t.Fatalf("Forward() = %v, quería ErrInvalidRequest", err)
			}
			if len(persister.requests) != 0 {
				t.Fatalf("llegó al Persistidor %d veces, quería 0: el rechazo es antes de salir", len(persister.requests))
			}
		})
	}
}

func TestForwardKeepsQueryAndRequestUntouched(t *testing.T) {
	persister := &proxyPersisterStub{response: domain.PersisterResponse{StatusCode: 200}}
	req := domain.PersisterRequest{
		Method:   "GET",
		Path:     "/api/v1/documents",
		RawQuery: "page=2&page_size=10&status=RESOLVED&include_deleted=true",
	}

	if _, err := NewProxyUseCase(persister).Forward(context.Background(), req); err != nil {
		t.Fatalf("Forward() = %v, quería nil", err)
	}
	if len(persister.requests) != 1 {
		t.Fatalf("llegó al Persistidor %d veces, quería 1", len(persister.requests))
	}
	if persister.requests[0] != req {
		t.Fatalf("reenvió %+v, quería el request tal cual %+v", persister.requests[0], req)
	}
}

func TestForwardReturnsResponseUntranslated(t *testing.T) {
	body := []byte(`{"document_id":"x"}`)
	persister := &proxyPersisterStub{response: domain.PersisterResponse{
		StatusCode: 200,
		Header:     map[string][]string{"Content-Type": {"application/json"}},
		Body:       body,
	}}

	resp, err := NewProxyUseCase(persister).Forward(context.Background(), domain.PersisterRequest{
		Method: "GET",
		Path:   "/api/v1/documents/" + demandDocumentID,
	})
	if err != nil {
		t.Fatalf("Forward() = %v, quería nil", err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != string(body) || resp.Header["Content-Type"][0] != "application/json" {
		t.Fatalf("respuesta = %+v, quería la que devolvió el Persistidor", resp)
	}
}

func TestForwardPropagatesTypedErrorsFromPersister(t *testing.T) {
	persister := &proxyPersisterStub{err: domain.ErrChecksumConflict}

	_, err := NewProxyUseCase(persister).Forward(context.Background(), domain.PersisterRequest{
		Method: "POST",
		Path:   "/api/v1/documents/" + demandDocumentID + "/restore",
	})
	if !errors.Is(err, domain.ErrChecksumConflict) {
		t.Fatalf("Forward() = %v, quería ErrChecksumConflict", err)
	}
}
