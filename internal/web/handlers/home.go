package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

// Home muestra el formulario de subida.
func (h *Handlers) Home(c *gin.Context) {
	h.renderer.HTML(c, http.StatusOK, "home.html", view.Home{
		Base:   flash(c, "upload"),
		MaxMiB: view.MaxUploadMiB,
	})
}

// homeError vuelve a mostrar el formulario con un mensaje y el código HTTP que corresponde.
func (h *Handlers) homeError(c *gin.Context, status int, message string, summarize bool) {
	h.renderer.HTML(c, status, "home.html", view.Home{
		Base:      view.Base{Nav: "upload", Error: message},
		Summarize: summarize,
		MaxMiB:    view.MaxUploadMiB,
	})
}
