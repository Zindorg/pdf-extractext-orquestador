package render

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Renderer struct {
	tmpl *template.Template
}

func New() (*Renderer, error) {
	t, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	return &Renderer{tmpl: t}, nil
}

func (r *Renderer) HTML(c *gin.Context, code int, name string, obj any) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if obj == nil {
		obj = gin.H{}
	}
	if err := r.tmpl.ExecuteTemplate(c.Writer, name, obj); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(code)
}
