package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// DownloadOriginal descarga el texto extraído.
func (h *Handlers) DownloadOriginal(c *gin.Context) { h.download(c, "original") }

// DownloadSummary descarga el resumen.
func (h *Handlers) DownloadSummary(c *gin.Context) { h.download(c, "summary") }

func (h *Handlers) download(c *gin.Context, kind string) {
	id := c.Param("id")
	resp, err := h.deps.Proxy.Forward(c.Request.Context(), domain.PersisterRequest{
		Method: http.MethodGet,
		Path:   "/api/v1/documents/" + id + "/download/" + kind,
	})
	if err != nil {
		// Una descarga fallida muestra una página con el motivo, no un cuerpo vacío.
		h.documentError(c, err, id)
		return
	}

	contentType := "text/plain; charset=utf-8"
	if ct := resp.Header["Content-Type"]; len(ct) > 0 && ct[0] != "" {
		contentType = ct[0]
	}
	if cd := resp.Header["Content-Disposition"]; len(cd) > 0 && cd[0] != "" {
		c.Header("Content-Disposition", cd[0])
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, contentType, resp.Body)
}
