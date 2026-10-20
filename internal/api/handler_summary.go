package api

import (
	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// handleSummary resuelve un resumen a demanda sobre un documento ya persistido. El documento no se
// re-sube: se lee y se descarga su texto por proxy, se resume y se publica `summary_resolved`.
func handleSummary(useCase SummaryUseCase, publicBaseURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		documentID := domain.DocumentID(c.Param("document_id"))

		result, err := useCase.Execute(c.Request.Context(), documentID)
		if err != nil {
			writeError(c, err)
			return
		}

		c.Header("X-Document-Id", string(result.DocumentID))
		c.Header("Location", documentLocation(publicBaseURL, result.DocumentID))
		c.JSON(202, newSummaryAccepted(result))
	}
}
