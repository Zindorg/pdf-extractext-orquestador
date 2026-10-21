package api

import (
	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// handleExtract admite un documento: valida el borde, corre el pipeline y responde `202` con el
// header `X-Document-Id` y una `Location` absoluta a la lectura.
func handleExtract(useCase ExtractUseCase, publicBaseURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := parseIntake(c)
		if err != nil {
			writeError(c, err)
			return
		}

		result, err := useCase.Execute(c.Request.Context(), domain.IncomingDocument{
			Bytes:     in.Document,
			Filename:  filenameOf(c),
			Summarize: in.Summarize,
			Metadata:  map[string]any{},
		})
		if err != nil {
			writeError(c, err)
			return
		}

		c.Header("X-Document-Id", string(result.DocumentID))
		c.Header("Location", documentLocation(publicBaseURL, result.DocumentID))
		c.JSON(202, newExtractAccepted(result))
	}
}

// filenameOf lee el nombre que declaró el cliente. Es un dato del cliente; el archivo no se guarda.
func filenameOf(c *gin.Context) string {
	form, err := c.MultipartForm()
	if err != nil {
		return ""
	}
	documents := form.File["document"]
	if len(documents) == 0 {
		return ""
	}
	return documents[0].Filename
}
