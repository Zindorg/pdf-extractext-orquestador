package middleware

import "github.com/gin-gonic/gin"

// ContextDocumentID es la clave con la que el document_id queda disponible para los middlewares
// posteriores (el log de acceso) y para los handlers. No hay un identificador de correlación
// separado: el document_id es el único (ADR-0004).
const ContextDocumentID = "document_id"

// Correlation expone el document_id de la ruta como header `X-Document-Id` en la respuesta y lo deja
// en el contexto para que el log de acceso lo registre en el mismo campo. Es el único correlador del
// sistema; una ruta sin documento no inventa uno (ADR-0004).
func Correlation() gin.HandlerFunc {
	return func(c *gin.Context) {
		documentID := c.Param("document_id")
		if documentID != "" {
			c.Header("X-Document-Id", documentID)
			c.Set(ContextDocumentID, documentID)
		}
		c.Next()
	}
}
