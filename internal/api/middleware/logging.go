package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog registra cada petición en una línea estructurada. Lleva el document_id en el mismo
// campo que todo lo demás cuando la ruta lo tiene: no hay un identificador de correlación aparte
// (ADR-0004). Si un handler adjuntó un error con c.Error, la línea lleva la causa y sube a nivel
// error cuando el status es 5xx: es la única traza de un fallo interno, porque el cuerpo de la
// respuesta no la expone. Usa el logger inyectado para no depender del logger global del proceso.
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
		if last := c.Errors.Last(); last != nil {
			attrs = append(attrs, "error", last.Err.Error())
		}
		if c.Writer.Status() >= http.StatusInternalServerError {
			logger.Error("acceso", attrs...)
			return
		}
		logger.Info("acceso", attrs...)
	}
}
