package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	weberrors "github.com/Zindorg/pdf-extractext-orquestador/internal/web/errors"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

// multipartOverhead es el margen sobre el tope del archivo para el resto del formulario multipart.
const multipartOverhead = 1 << 20

// Upload recibe el PDF, lo procesa y redirige al detalle del documento.
func (h *Handlers) Upload(c *gin.Context) {
	// Tope duro sobre el cuerpo: sin él, un archivo enorme se leería (y se volcaría a disco) antes de
	// poder rechazarlo.
	maxUploadBytes := h.maxUploadBytes()
	tooLarge := fmt.Sprintf("El archivo supera los %d MiB.", h.maxUploadMiB())
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxUploadBytes)+multipartOverhead)

	file, header, err := c.Request.FormFile("document")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			h.homeError(c, http.StatusRequestEntityTooLarge, tooLarge, false)
			return
		}
		h.homeError(c, http.StatusBadRequest, "Seleccioná un archivo PDF para subir.", false)
		return
	}
	defer func() { _ = file.Close() }()

	summarize := c.PostForm("summarize") == "true"

	data, err := io.ReadAll(io.LimitReader(file, int64(maxUploadBytes)+1))
	if err != nil {
		h.homeError(c, http.StatusBadRequest, "No se pudo leer el archivo.", summarize)
		return
	}
	switch {
	case len(data) == 0:
		h.homeError(c, http.StatusBadRequest, "El archivo está vacío.", summarize)
		return
	case len(data) > maxUploadBytes:
		h.homeError(c, http.StatusRequestEntityTooLarge, tooLarge, summarize)
		return
	}

	result, err := h.deps.Extract.Execute(c.Request.Context(), domain.IncomingDocument{
		Bytes:     data,
		Filename:  cleanFilename(header.Filename),
		Summarize: summarize,
		Metadata:  map[string]any{},
	})
	if err != nil {
		_ = c.Error(err)
		var de *domain.Error
		if !errors.As(err, &de) {
			h.log().Error("fallo al procesar un documento", "error", err)
		}
		h.homeError(c, weberrors.StatusCode(err), weberrors.UserMessage(err), summarize)
		return
	}

	id := string(result.DocumentID)
	if result.DeliveryStatus == domain.DeliveryAvailable {
		// El contenido ya estaba guardado: el documento se puede consultar ya mismo.
		redirect(c, "/documents/"+id+"?notice=duplicate")
		return
	}
	// Recién publicado: el Persistidor lo asienta de forma asíncrona. La página de detalle espera y
	// se recarga sola hasta que aparece (el 404 inicial es transitorio, no un error).
	redirect(c, "/documents/"+id+"?notice=uploaded&wait="+view.WaitDoc+"&n=0")
}

// cleanFilename deja solo el nombre del archivo, sin la ruta que algunos navegadores antiguos envían,
// y con un largo razonable.
func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" {
		return ""
	}
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	return name
}
