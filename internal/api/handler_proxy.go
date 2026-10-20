package api

import (
	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// handleProxy reenvía una petición al Persistidor tal como llegó. El prefijo `/api/v1` se conserva
// (ADR-0005) y la respuesta vuelve sin traducir: es un proxy transparente.
func handleProxy(useCase ProxyUseCase) gin.HandlerFunc {
	return func(c *gin.Context) {
		response, err := useCase.Forward(c.Request.Context(), domain.PersisterRequest{
			Method:   c.Request.Method,
			Path:     c.Request.URL.Path,
			RawQuery: c.Request.URL.RawQuery,
		})
		if err != nil {
			writeError(c, err)
			return
		}

		for name, values := range response.Header {
			for _, value := range values {
				c.Writer.Header().Add(name, value)
			}
		}
		c.Data(response.StatusCode, c.Writer.Header().Get("Content-Type"), response.Body)
	}
}
