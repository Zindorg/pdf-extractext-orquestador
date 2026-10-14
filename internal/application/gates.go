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
