package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recovery evita que un panic tumbe el servicio público: lo captura, lo registra y responde `500`
// con el envoltorio de error. El cuerpo no expone el mensaje del panic.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic en el handler", "error", recovered, "path", c.Request.URL.Path)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    "internal",
					"message": "error interno",
				})
			}
		}()
		c.Next()
	}
}
