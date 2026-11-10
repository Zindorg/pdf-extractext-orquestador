package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// handleExtract admite un documento: valida el borde, corre el pipeline y responde `202` con el
// header `X-Document-Id` y una `Location` absoluta a la lectura.
func handleExtract(useCase ExtractUseCase, publicBaseURL string, maxBytes int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Tope duro sobre el cuerpo: sin él, un archivo enorme se leería entero (y gin lo volcaría a
		// disco) antes de poder rechazarlo.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxBytes)+multipartOverhead)

		in, err := parseIntake(c, maxBytes)
		if err != nil {
			writeError(c, err)
			return
		}

		result, err := useCase.Execute(c.Request.Context(), domain.IncomingDocument{
			Bytes:     in.Document,
			Filename:  in.Filename,
			Summarize: in.Summarize,
			Metadata:  map[string]any{},
		})
		if err != nil {
			writeError(c, err)
			return
		}

		c.Header("X-Document-Id", string(result.DocumentID))
		c.Header("Location", documentLocation(publicBaseURL, result.DocumentID))
		c.JSON(http.StatusAccepted, newExtractAccepted(result))
	}
}
