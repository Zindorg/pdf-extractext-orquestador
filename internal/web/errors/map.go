// Package errors mapea los códigos de error del Orquestador a mensajes en español para la capa web.
package errors

import (
	"net/http"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// UserMessage devuelve un mensaje legible en español para un error de dominio.
// Nunca expone detalles internos ni el mensaje crudo del extractor.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var de *domain.Error
	if ok := isDomainErr(err, &de); !ok {
		return "Ocurrió un error inesperado. Intentá nuevamente."
	}

	switch de.Code {
	case "invalid_request":
		return de.Message
	case "payload_too_large":
		return "El archivo supera los 10 MiB."
	case "unsupported_media_type":
		return "El archivo no es un PDF válido (debe comenzar con %PDF-)."
	case "extraction_failed":
		if de.Reason == domain.ReasonEncryptedPDF || de.Reason == domain.ReasonPasswordProtected {
			return "El PDF está protegido con contraseña."
		}
		return "No se pudo extraer el documento."
	case "concurrency_limited":
		return "Se alcanzó el límite de concurrencia. Intentá nuevamente en unos segundos."
	case "summary_failed":
		return "No se pudo generar el resumen. Probá de nuevo más tarde."
	case "extractor_unavailable":
		return "El servicio de extracción no está disponible en este momento. Probá nuevamente."
	case "document_not_found":
		return "El documento no está disponible."
	case "summary_not_available":
		return "Este documento no tiene resumen o no se solicitó su generación."
	case "summary_pending":
		return "El resumen todavía está en curso."
	case "summary_already_resolved":
		return "Este documento ya tiene un resumen resuelto."
	case "checksum_conflict":
		return "No se puede restaurar porque ya existe un documento activo con el mismo contenido."
	case "rate_limited":
		return "Límite de solicitudes alcanzado. Intentá nuevamente más tarde."
	case "internal":
		return "Ocurrió un error inesperado. Intentá nuevamente."
	default:
		return "Ocurrió un error inesperado. Intentá nuevamente."
	}
}

// StatusCode intenta mapear el código de dominio a un status HTTP para casos donde sea necesario.
func StatusCode(err error) int {
	var de *domain.Error
	if ok := isDomainErr(err, &de); !ok {
		return http.StatusInternalServerError
	}
	switch de.Code {
	case "invalid_request":
		return http.StatusBadRequest
	case "payload_too_large":
		return http.StatusRequestEntityTooLarge
	case "unsupported_media_type":
		return http.StatusUnsupportedMediaType
	case "extraction_failed":
		return http.StatusUnprocessableEntity
	case "concurrency_limited", "rate_limited":
		return http.StatusTooManyRequests
	case "summary_failed":
		return http.StatusBadGateway
	case "extractor_unavailable":
		return http.StatusServiceUnavailable
	case "document_not_found", "summary_not_available":
		return http.StatusNotFound
	case "summary_pending", "summary_already_resolved", "checksum_conflict":
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func isDomainErr(err error, target **domain.Error) bool {
	// simple type assertion style
	if e, ok := err.(*domain.Error); ok {
		*target = e
		return true
	}
	return false
}
