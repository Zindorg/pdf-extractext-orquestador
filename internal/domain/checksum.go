package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// NewChecksum calcula el sha256 del texto extraído, en hex minúsculas (ADR-0003).
// Es la identidad del contenido: dos documentos con el mismo checksum son duplicados.
func NewChecksum(text string) Checksum {
	sum := sha256.Sum256([]byte(text))
	return Checksum(hex.EncodeToString(sum[:]))
}
