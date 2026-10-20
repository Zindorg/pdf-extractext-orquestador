package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleHealth es el liveness: responde `200` mientras el proceso pueda atender. No consulta
// dependencias —un Extractor caído no debe reiniciar el servicio— y no declara `500`.
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleReady es el readiness: responde `503` cuando el servicio no debería recibir tráfico. Con
// Redis caído no se puede publicar en el stream, así que admitir documentos solo produciría errores.
func handleReady(ready func(ctx context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ready != nil {
			if err := ready(c.Request.Context()); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "reason": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
