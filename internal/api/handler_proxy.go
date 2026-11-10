package api

import (
	"net/http"

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

		header := http.Header(response.Header)
		for _, name := range forwardedHeaders {
			if value := header.Get(name); value != "" {
				c.Header(name, value)
			}
		}
		// El cuerpo puede ser texto del usuario: el navegador no debe adivinar otro tipo.
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(response.StatusCode, header.Get("Content-Type"), response.Body)
	}
}

// forwardedHeaders son las cabeceras del Persistidor que llegan al cliente. Las demás —hop-by-hop
// como Connection o Transfer-Encoding, Set-Cookie, Content-Length (la recalcula gin)— no se copian.
var forwardedHeaders = []string{"Content-Type", "Content-Disposition", "ETag", "Last-Modified", "Cache-Control"}
