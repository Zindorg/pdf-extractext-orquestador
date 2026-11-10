package application

import (
	"bytes"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// DefaultMaxDocumentBytes es el techo del archivo cuando la configuración no fija otro
// (MAX_DOCUMENT_BYTES). Se aplica antes de gastar CPU en el Extractor.
const DefaultMaxDocumentBytes = 10 << 20

var pdfSignature = []byte("%PDF-")

// mimeTypePDF es el único tipo que pasa la puerta 1: la firma lo determina, no lo que declare el
// cliente.
const mimeTypePDF = "application/pdf"

// validateDocument es la puerta 1: admite el documento solo si no supera maxBytes y empieza con la
// firma de PDF.
func validateDocument(size int, head []byte, maxBytes int) error {
	if size > maxBytes {
		return domain.ErrDocumentTooLarge
	}
	if !bytes.HasPrefix(head, pdfSignature) {
		return domain.ErrUnsupportedMediaType
	}
	return nil
}

// classifyExtractionReason es la puerta 2: admite la extracción solo si el motivo es `success`.
// Cualquier otro motivo del enum se rechaza con 422; uno fuera del enum es un fallo del Extractor,
// para no inventar una categoría que el servicio no declaró.
func classifyExtractionReason(reason domain.ExtractionReason) error {
	switch reason {
	case domain.ReasonSuccess:
		return nil
	case domain.ReasonEmpty,
		domain.ReasonEncryptedPDF,
		domain.ReasonPasswordProtected,
		domain.ReasonCorrupt,
		domain.ReasonUnsupportedFeatures:
		return domain.NewExtractionFailed(reason)
	default:
		return domain.ErrExtractorUnavailable
	}
}
