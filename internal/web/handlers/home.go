package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) Home(c *gin.Context) {
	h.renderer.HTML(c, http.StatusOK, "home.html", gin.H{"CSRFToken": ""})
}
