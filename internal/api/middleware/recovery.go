package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery evita que un panic tumbe el servicio público: lo captura, lo registra y responde `500`
// con el envoltorio de error. El cuerpo no expone el mensaje del panic. Usa el logger por defecto,
// que main fija al logger JSON del servicio. Si la respuesta ya empezó a escribirse no se puede
// cambiar el status: solo se corta.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic en el handler", "error", recovered, "path", c.Request.URL.Path, "stack", string(debug.Stack()))
				if c.Writer.Written() {
					c.Abort()
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    "internal",
					"message": "error interno",
				})
			}
		}()
		c.Next()
	}
}
