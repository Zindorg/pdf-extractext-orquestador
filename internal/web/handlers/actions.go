package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

// Las acciones responden siempre con una redirección (POST → redirect → GET): recargar la página
// resultante no repite la acción, y el resultado se muestra como aviso en la página de destino.

// RequestSummary pide el resumen de un documento ya guardado.
func (h *Handlers) RequestSummary(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.deps.Summary.Execute(c.Request.Context(), domain.DocumentID(id)); err != nil {
		_ = c.Error(err)
		redirect(c, "/documents/"+id+"?error="+errCode(err))
		return
	}
	// El resumen se publicó, pero el Persistidor lo asienta de forma asíncrona: la página espera.
	redirect(c, "/documents/"+id+"?notice=summary_requested&wait="+view.WaitSummary+"&n=0")
}

// Delete borra (lógicamente) un documento.
func (h *Handlers) Delete(c *gin.Context) {
	id := c.Param("id")
	_, err := h.deps.Proxy.Forward(c.Request.Context(), domain.PersisterRequest{
		Method: http.MethodDelete,
		Path:   "/api/v1/documents/" + id,
	})
	if err != nil {
		_ = c.Error(err)
		redirect(c, "/documents/"+id+"?error="+errCode(err))
		return
	}
	redirect(c, "/documents?notice=deleted")
}

// Restore restaura un documento borrado.
func (h *Handlers) Restore(c *gin.Context) {
	id := c.Param("id")
	_, err := h.deps.Proxy.Forward(c.Request.Context(), domain.PersisterRequest{
		Method: http.MethodPost,
		Path:   "/api/v1/documents/" + id + "/restore",
	})
	if err != nil {
		// Un documento borrado no se puede abrir: el error se muestra donde está la acción.
		_ = c.Error(err)
		redirect(c, "/documents?include_deleted=true&error="+errCode(err))
		return
	}
	redirect(c, "/documents/"+id+"?notice=restored")
}
