// Package weberrors mapea los códigos de error del Orquestador a mensajes en español para la capa web.
package weberrors

import (
	stderrors "errors"
	"net/http"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/httpstatus"
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

	switch de.Code() {
	case "invalid_request":
		return de.Message()
	case "payload_too_large":
		return "El archivo supera el tamaño máximo permitido."
	case "unsupported_media_type":
		return "El archivo no es un PDF válido (debe comenzar con %PDF-)."
	case "extraction_failed":
		if de.Reason() == domain.ReasonEncryptedPDF || de.Reason() == domain.ReasonPasswordProtected {
			return "El PDF está protegido con contraseña."
		}
		return "No se pudo extraer el documento."
	case "concurrency_limited":
		return "Se alcanzó el límite de concurrencia. Intentá nuevamente en unos segundos."
	case "summary_failed":
		return "No se pudo generar el resumen. Probá de nuevo más tarde."
	case "extractor_unavailable":
		return "El servicio de extracción no está disponible en este momento. Probá nuevamente."
	case "persister_unavailable":
		return "El servicio de documentos no respondió. Probá de nuevo en unos segundos."
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

// StatusCode devuelve el status HTTP de un error de dominio con la misma tabla que la API
// (httpstatus); un error que no es de dominio o un código desconocido es 500.
func StatusCode(err error) int {
	var de *domain.Error
	if !isDomainErr(err, &de) {
		return http.StatusInternalServerError
	}
	if status, ok := httpstatus.ForCode(de.Code()); ok {
		return status
	}
	return http.StatusInternalServerError
}

// isDomainErr reconoce un error de dominio aunque venga envuelto: los adaptadores lo envuelven con
// la causa (fmt.Errorf("%w: ...")) y una aserción de tipo directa lo perdería.
func isDomainErr(err error, target **domain.Error) bool {
	return stderrors.As(err, target)
}
