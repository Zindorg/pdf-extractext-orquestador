package weberrors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestCadaCodigoTieneMensajeYStatus(t *testing.T) {
	generic := UserMessage(errors.New("x"))
	cases := []struct {
		err    error
		status int
	}{
		{domain.ErrInvalidRequest, http.StatusBadRequest},
		{domain.ErrDocumentTooLarge, http.StatusRequestEntityTooLarge},
		{domain.ErrUnsupportedMediaType, http.StatusUnsupportedMediaType},
		{domain.NewExtractionFailed(domain.ReasonCorrupt), http.StatusUnprocessableEntity},
		{domain.ErrSummaryFailed, http.StatusBadGateway},
		{domain.ErrExtractorUnavailable, http.StatusServiceUnavailable},
		{domain.ErrPersisterUnavailable, http.StatusServiceUnavailable},
		{domain.ErrDocumentNotFound, http.StatusNotFound},
		{domain.ErrSummaryNotAvailable, http.StatusNotFound},
		{domain.ErrSummaryPending, http.StatusConflict},
		{domain.ErrSummaryAlreadyResolved, http.StatusConflict},
		{domain.ErrChecksumConflict, http.StatusConflict},
		{domain.ErrRateLimited, http.StatusTooManyRequests},
		{domain.NewError("concurrency_limited", "x"), http.StatusTooManyRequests},
	}
	for _, tc := range cases {
		if got := StatusCode(tc.err); got != tc.status {
			t.Errorf("StatusCode(%v) = %d, quería %d", tc.err, got, tc.status)
		}
		if msg := UserMessage(tc.err); msg == "" || msg == generic {
			t.Errorf("UserMessage(%v) = %q: falta un mensaje propio", tc.err, msg)
		}
	}
}

func TestErroresDesconocidosSonGenericos(t *testing.T) {
	for _, err := range []error{errors.New("boom"), domain.NewError("inventado", "x")} {
		if StatusCode(err) != http.StatusInternalServerError {
			t.Errorf("StatusCode(%v) = %d, quería 500", err, StatusCode(err))
		}
	}
	if UserMessage(nil) != "" {
		t.Error("sin error no hay mensaje")
	}
}
