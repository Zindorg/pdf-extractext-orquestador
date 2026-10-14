package application

import (
	"bytes"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// maxDocumentBytes es el techo del archivo, aplicado antes de gastar CPU en el Extractor.
const maxDocumentBytes = 10 << 20

var pdfSignature = []byte("%PDF-")

// validateDocument es la puerta 1: admite el documento solo si entra en el tope y empieza con la
// firma de PDF. Toma el tamaño y la cabecera en vez del archivo entero para no necesitar tenerlo
// completo en memoria.
func validateDocument(size int, head []byte) error {
	if size > maxDocumentBytes {
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
