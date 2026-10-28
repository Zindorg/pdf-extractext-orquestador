package errors

import (
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestUserMessagePasswordProtected(t *testing.T) {
	err := domain.NewExtractionFailed(domain.ReasonPasswordProtected)
	if got := UserMessage(err); got != "El PDF está protegido con contraseña." {
		t.Fatalf("got %q", got)
	}
}

func TestUserMessageSummaryFailed(t *testing.T) {
	if got := UserMessage(domain.ErrSummaryFailed); got == "" {
		t.Fatalf("empty")
	}
}
