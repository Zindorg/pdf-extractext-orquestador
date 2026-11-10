package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/httpstatus"
)

// errorBody es el envoltorio único de error (openapi.yaml, esquema `Error`). Todos los errores
// tienen la misma forma para que el cliente distinga el motivo por `code` y no parseando el mensaje.
type errorBody struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// errorResponse traduce un error a su status HTTP y su envoltorio. Un error de dominio lleva su
// propio `code` y su status sale de la tabla única (httpstatus); cualquier otro error se responde `500` con `internal`
// y un mensaje fijo: el cuerpo no expone detalles internos del servidor.
func errorResponse(err error) (int, errorBody) {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		status, ok := httpstatus.ForCode(domainErr.Code())
		if !ok {
			return http.StatusInternalServerError, internalBody()
		}
		return status, errorBody{
			Code:    domainErr.Code(),
			Message: domainErr.Message(),
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
	if domainErr.Reason() == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"reason": string(domainErr.Reason())})
	if err != nil {
		return nil
	}
	return payload
}

// writeError responde un error con su status y su envoltorio JSON. El error original se adjunta al
// contexto de gin para que el log de acceso registre la causa: el cuerpo no la expone, el log sí.
func writeError(c *gin.Context, err error) {
	_ = c.Error(err)
	status, body := errorResponse(err)
	c.JSON(status, body)
}
