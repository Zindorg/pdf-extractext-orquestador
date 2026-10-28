package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	weberrors "github.com/Zindorg/pdf-extractext-orquestador/internal/web/errors"
)

func (h *Handlers) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("document")
	if err != nil {
		h.renderer.HTML(c, http.StatusBadRequest, "home.html", gin.H{"Error": "Debe seleccionar un archivo PDF."})
		return
	}
	defer file.Close()

	if header.Size > 10<<20 {
		h.renderer.HTML(c, http.StatusRequestEntityTooLarge, "home.html", gin.H{"Error": "El archivo supera los 10 MiB."})
		return
	}
	buf := make([]byte, header.Size)
	_, err = io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		h.renderer.HTML(c, http.StatusBadRequest, "home.html", gin.H{"Error": "No se pudo leer el archivo."})
		return
	}

	summarize := c.PostForm("summarize") == "true"

	result, err := h.deps.Extract.Execute(c.Request.Context(), domain.IncomingDocument{
		Bytes:     buf,
		Filename:  header.Filename,
		Summarize: summarize,
		Metadata:  map[string]any{},
	})
	if err != nil {
		h.renderer.HTML(c, weberrors.StatusCode(err), "home.html", gin.H{"Error": weberrors.UserMessage(err)})
		return
	}

	c.Redirect(http.StatusFound, "/documents/"+string(result.DocumentID))
}
