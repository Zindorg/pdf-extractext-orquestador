package handlers

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func (h *Handlers) DownloadOriginal(c *gin.Context) {
	id := c.Param("id")
	req := domain.PersisterRequest{Method: "GET", Path: "/api/v1/documents/" + id + "/download/original"}
	resp, err := h.deps.Proxy.Forward(c.Request.Context(), req)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if ct, ok := resp.Header["Content-Type"]; ok && len(ct) > 0 {
		c.Header("Content-Type", ct[0])
	}
	if cd, ok := resp.Header["Content-Disposition"]; ok && len(cd) > 0 {
		c.Header("Content-Disposition", cd[0])
	}
	c.Status(resp.StatusCode)
	if len(resp.Body) > 0 {
		_, _ = c.Writer.Write(resp.Body)
	}
}

func (h *Handlers) DownloadSummary(c *gin.Context) {
	id := c.Param("id")
	req := domain.PersisterRequest{Method: "GET", Path: "/api/v1/documents/" + id + "/download/summary"}
	resp, err := h.deps.Proxy.Forward(c.Request.Context(), req)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if ct, ok := resp.Header["Content-Type"]; ok && len(ct) > 0 {
		c.Header("Content-Type", ct[0])
	}
	if cd, ok := resp.Header["Content-Disposition"]; ok && len(cd) > 0 {
		c.Header("Content-Disposition", cd[0])
	}
	c.Status(resp.StatusCode)
	if len(resp.Body) > 0 {
		_, _ = bytes.NewBuffer(resp.Body).WriteTo(c.Writer)
	}
}
