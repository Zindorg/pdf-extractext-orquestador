package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// errorBody es el envoltorio único de error (openapi.yaml, esquema `Error`). Todos los errores
// tienen la misma forma para que el cliente distinga el motivo por `code` y no parseando el mensaje.
type errorBody struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// errorResponse traduce un error a su status HTTP y su envoltorio. Un error de dominio lleva su
// propio `code` y su status en `statusByCode`; cualquier otro error se responde `500` con `internal`
// y un mensaje fijo: el cuerpo no expone detalles internos del servidor.
func errorResponse(err error) (int, errorBody) {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		status, ok := statusByCode[domainErr.Code]
		if !ok {
			return http.StatusInternalServerError, internalBody()
		}
		return status, errorBody{
			Code:    domainErr.Code,
			Message: domainErr.Message,
			Details: detailsFor(domainErr),
		}
	}
	return http.StatusInternalServerError, internalBody()
}

func internalBody() errorBody {
	return errorBody{Code: "internal", Message: "error interno"}
}

// detailsFor arma los detalles estructurados de un error de dominio. Solo la extracción fallida
// lleva uno: el motivo del enum del Extractor, que viaja en `details.reason`.
func detailsFor(domainErr *domain.Error) json.RawMessage {
	if domainErr.Reason == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"reason": string(domainErr.Reason)})
	if err != nil {
		return nil
	}
	return payload
}

// statusByCode mapea el vocabulario público de códigos al status HTTP (tasks/fase7.md). El dominio
// no conoce códigos HTTP: esta es la única traducción entre los dos mundos.
var statusByCode = map[string]int{
	"invalid_request":          http.StatusBadRequest,
	"document_not_found":       http.StatusNotFound,
	"summary_not_available":    http.StatusNotFound,
	"summary_pending":          http.StatusConflict,
	"summary_already_resolved": http.StatusConflict,
	"checksum_conflict":        http.StatusConflict,
	"extraction_failed":        http.StatusUnprocessableEntity,
	"summary_failed":           http.StatusBadGateway,
	"extractor_unavailable":    http.StatusServiceUnavailable,
	"payload_too_large":        http.StatusRequestEntityTooLarge,
	"unsupported_media_type":   http.StatusUnsupportedMediaType,
}

// writeError responde un error con su status y su envoltorio JSON.
func writeError(c *gin.Context, err error) {
	status, body := errorResponse(err)
	c.JSON(status, body)
}
