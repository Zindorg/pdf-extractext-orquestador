// Package httpstatus traduce el vocabulario público de códigos de error (openapi.yaml, `ErrorCode`)
// a status HTTP. Es la única tabla del servicio: la usan la API y la interfaz web, para que un mismo
// error no responda con un status distinto según por dónde entró.
package httpstatus

import "net/http"

var byCode = map[string]int{
	"invalid_request":          http.StatusBadRequest,
	"document_not_found":       http.StatusNotFound,
	"summary_not_available":    http.StatusNotFound,
	"summary_pending":          http.StatusConflict,
	"summary_already_resolved": http.StatusConflict,
	"checksum_conflict":        http.StatusConflict,
	"payload_too_large":        http.StatusRequestEntityTooLarge,
	"unsupported_media_type":   http.StatusUnsupportedMediaType,
	"extraction_failed":        http.StatusUnprocessableEntity,
	"rate_limited":             http.StatusTooManyRequests,
	"concurrency_limited":      http.StatusTooManyRequests,
	"internal":                 http.StatusInternalServerError,
	"summary_failed":           http.StatusBadGateway,
	"extractor_unavailable":    http.StatusServiceUnavailable,
	"persister_unavailable":    http.StatusServiceUnavailable,
}

// ForCode devuelve el status HTTP de un código público y si el código pertenece al vocabulario.
func ForCode(code string) (int, bool) {
	status, ok := byCode[code]
	return status, ok
}
