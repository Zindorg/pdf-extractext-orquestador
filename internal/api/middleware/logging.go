package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog registra cada petición en una línea estructurada. Lleva el document_id en el mismo
// campo que todo lo demás cuando la ruta lo tiene: no hay un identificador de correlación aparte
// (ADR-0004). Usa el logger inyectado para no depender del logger global del proceso.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()

		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		}
		if documentID, ok := c.Get(ContextDocumentID); ok {
			attrs = append(attrs, "document_id", documentID)
		}
		logger.Info("acceso", attrs...)
	}
}
