package application

import (
	"errors"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestValidateDocument(t *testing.T) {
	pdfHead := []byte("%PDF-1.7")

	cases := []struct {
		name string
		size int
		head []byte
		want error
	}{
		{"pdf dentro del tope", 5, pdfHead, nil},
		{"justo en el tope", maxDocumentBytes, pdfHead, nil},
		{"un byte por encima", maxDocumentBytes + 1, pdfHead, domain.ErrDocumentTooLarge},
		{"sin firma pdf", 5, []byte("PK\x03\x04"), domain.ErrUnsupportedMediaType},
		{"firma incompleta", 3, []byte("%PD"), domain.ErrUnsupportedMediaType},
		{"sin cabecera", 0, nil, domain.ErrUnsupportedMediaType},
		{"grande y sin firma pesa el tamaño", maxDocumentBytes + 1, []byte("PK\x03\x04"), domain.ErrDocumentTooLarge},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateDocument(tc.size, tc.head)
			if !errors.Is(got, tc.want) {
				t.Fatalf("validateDocument(%d, %q) = %v, quería %v", tc.size, tc.head, got, tc.want)
			}
		})
	}
}
