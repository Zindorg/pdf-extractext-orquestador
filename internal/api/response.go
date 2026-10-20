package api

import (
	"encoding/json"
	"strings"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// documentLocation arma la URI absoluta de lectura del documento. Es absoluta (RFC 7231 §7.1.2)
// porque el servicio conoce su path público (ADR-0005) y así el cliente no compone la URL.
func documentLocation(publicBaseURL string, documentID domain.DocumentID) string {
	return strings.TrimRight(publicBaseURL, "/") + "/api/v1/documents/" + string(documentID)
}

// decodeJSON es un helper de decodificación usado por los tests.
func decodeJSON(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
