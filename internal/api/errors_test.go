package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestErrorResponseMapsDomainErrors(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantReason  domain.ExtractionReason
		wantDetails bool
	}{
		{"documento demasiado grande", domain.ErrDocumentTooLarge, http.StatusRequestEntityTooLarge, "payload_too_large", "", false},
		{"tipo no soportado", domain.ErrUnsupportedMediaType, http.StatusUnsupportedMediaType, "unsupported_media_type", "", false},
		{"extractor caído", domain.ErrExtractorUnavailable, http.StatusServiceUnavailable, "extractor_unavailable", "", false},
		{"resumidor caído", domain.ErrSummaryFailed, http.StatusBadGateway, "summary_failed", "", false},
		{"petición inválida", domain.ErrInvalidRequest, http.StatusBadRequest, "invalid_request", "", false},
		{"documento ausente", domain.ErrDocumentNotFound, http.StatusNotFound, "document_not_found", "", false},
		{"resumen pendiente", domain.ErrSummaryPending, http.StatusConflict, "summary_pending", "", false},
		{"resumen ya resuelto", domain.ErrSummaryAlreadyResolved, http.StatusConflict, "summary_already_resolved", "", false},
		{"resumen no disponible", domain.ErrSummaryNotAvailable, http.StatusNotFound, "summary_not_available", "", false},
		{"checksum ocupado", domain.ErrChecksumConflict, http.StatusConflict, "checksum_conflict", "", false},
		{"extracción fallida", domain.NewExtractionFailed(domain.ReasonEncryptedPDF), http.StatusUnprocessableEntity, "extraction_failed", domain.ReasonEncryptedPDF, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := errorResponse(tc.err)

			if status != tc.wantStatus {
				t.Fatalf("status = %d, quería %d", status, tc.wantStatus)
			}
			if body.Code != tc.wantCode {
				t.Fatalf("code = %q, quería %q", body.Code, tc.wantCode)
			}
			if body.Message == "" {
				t.Fatal("el mensaje no puede ir vacío")
			}
			if tc.wantDetails {
				var details map[string]any
				if err := json.Unmarshal(body.Details, &details); err != nil {
					t.Fatalf("details ilegible: %v", err)
				}
				if details["reason"] != string(tc.wantReason) {
					t.Fatalf("details.reason = %v, quería %q", details["reason"], tc.wantReason)
				}
			}
		})
	}
}

func TestErrorResponseHidesUnexpectedErrors(t *testing.T) {
	status, body := errorResponse(errors.New("fallo interno con detalles que no se exponen"))

	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", status)
	}
	if body.Code != "internal" {
		t.Fatalf("code = %q, quería internal", body.Code)
	}
	if body.Message == "fallo interno con detalles que no se exponen" {
		t.Fatal("el cuerpo no debe exponer el error interno")
	}
}

func TestErrorResponseHandlesWrappedDomainError(t *testing.T) {
	wrapped := errors.New("contexto de transporte")
	combined := errors.Join(domain.ErrExtractorUnavailable, wrapped)

	status, body := errorResponse(combined)
	if status != http.StatusServiceUnavailable || body.Code != "extractor_unavailable" {
		t.Fatalf("error envuelto = %d/%q, quería 503/extractor_unavailable", status, body.Code)
	}
}
