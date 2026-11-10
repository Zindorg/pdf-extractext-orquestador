package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestFromCodeSoloReconoceElVocabularioDelPersistidor(t *testing.T) {
	for _, want := range []*Error{ErrInvalidRequest, ErrDocumentNotFound, ErrSummaryPending, ErrChecksumConflict, ErrSummaryNotAvailable, ErrRateLimited} {
		if got := FromCode(want.Code()); got != want {
			t.Errorf("FromCode(%q) = %v", want.Code(), got)
		}
	}
	for _, code := range []string{"", "inventado", "internal", "extractor_unavailable"} {
		if FromCode(code) != nil {
			t.Errorf("FromCode(%q) no debe traducirse", code)
		}
	}
}

func TestErrorExponeSusDatosSoloPorLectura(t *testing.T) {
	err := NewExtractionFailed(ReasonEncryptedPDF)
	if err.Code() != "extraction_failed" || err.Reason() != ReasonEncryptedPDF || err.Message() == "" || err.Error() != err.Message() {
		t.Fatalf("error = %#v", err)
	}
	wrapped := fmt.Errorf("contexto: %w", ErrPersisterUnavailable)
	var de *Error
	if !errors.As(wrapped, &de) || de.Code() != "persister_unavailable" {
		t.Fatalf("errors.As no recuperó el error envuelto: %v", wrapped)
	}
}
