package errors

import (
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestUserMessageExtractionFailedEncrypted(t *testing.T) {
	err := domain.NewExtractionFailed(domain.ReasonEncryptedPDF)
	got := UserMessage(err)
	if got != "El PDF está protegido con contraseña." {
		t.Fatalf("got %q", got)
	}
}

func TestUserMessageExtractionFailedUnknownReason(t *testing.T) {
	err := domain.NewExtractionFailed(domain.ReasonCorrupt)
	got := UserMessage(err)
	if got != "No se pudo extraer el documento." {
		t.Fatalf("got %q", got)
	}
}

func TestUserMessagePayloadTooLarge(t *testing.T) {
	if got := UserMessage(domain.ErrDocumentTooLarge); got != "El archivo supera los 10 MiB." {
		t.Fatalf("got %q", got)
	}
}

func TestUserMessageUnsupportedMediaType(t *testing.T) {
	if got := UserMessage(domain.ErrUnsupportedMediaType); got != "El archivo no es un PDF válido (debe comenzar con %PDF-)." {
		t.Fatalf("got %q", got)
	}
}

func TestUserMessageSummaryPending(t *testing.T) {
	if got := UserMessage(domain.ErrSummaryPending); got != "El resumen todavía está en curso." {
		t.Fatalf("got %q", got)
	}
}

func TestUserMessageSummaryNotAvailable(t *testing.T) {
	if got := UserMessage(domain.ErrSummaryNotAvailable); got != "Este documento no tiene resumen o no se solicitó su generación." {
		t.Fatalf("got %q", got)
	}
}
