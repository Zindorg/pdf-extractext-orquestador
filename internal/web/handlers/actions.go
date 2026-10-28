package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	weberrors "github.com/Zindorg/pdf-extractext-orquestador/internal/web/errors"
)

func (h *Handlers) RequestSummary(c *gin.Context) {
	id := c.Param("id")
	res, err := h.deps.Summary.Execute(c.Request.Context(), domain.DocumentID(id))
	if err != nil {
		h.renderer.HTML(c, weberrors.StatusCode(err), "document_detail.html", gin.H{"Error": weberrors.UserMessage(err)})
		return
	}
	_ = res
	c.Redirect(http.StatusFound, "/documents/"+id)
}

func (h *Handlers) Delete(c *gin.Context) {
	id := c.Param("id")
	req := domain.PersisterRequest{Method: "DELETE", Path: "/api/v1/documents/" + id}
	_, err := h.deps.Proxy.Forward(c.Request.Context(), req)
	if err != nil {
		h.renderer.HTML(c, weberrors.StatusCode(err), "document_detail.html", gin.H{"Error": weberrors.UserMessage(err)})
		return
	}
	c.Redirect(http.StatusFound, "/documents")
}

func (h *Handlers) Restore(c *gin.Context) {
	id := c.Param("id")
	req := domain.PersisterRequest{Method: "POST", Path: "/api/v1/documents/" + id + "/restore"}
	_, err := h.deps.Proxy.Forward(c.Request.Context(), req)
	if err != nil {
		h.renderer.HTML(c, weberrors.StatusCode(err), "document_detail.html", gin.H{"Error": weberrors.UserMessage(err)})
		return
	}
	c.Redirect(http.StatusFound, "/documents/"+id)
}
