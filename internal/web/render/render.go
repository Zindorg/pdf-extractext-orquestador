package render

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Renderer genera las páginas HTML.
type Renderer struct {
	pages map[string]*template.Template
}

// New carga y valida todas las plantillas: un error de sintaxis falla al arrancar, no en la primera
// petición.
func New() (*Renderer, error) {
	pages, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	return &Renderer{pages: pages}, nil
}

// Execute genera la página name con data. Renderiza a memoria primero: si la plantilla falla a mitad,
// el cliente recibe un error limpio y no una página cortada con estado 200.
func (r *Renderer) Execute(name string, data any) ([]byte, error) {
	t, ok := r.pages[name]
	if !ok {
		return nil, fmt.Errorf("plantilla %q inexistente", name)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base.html", data); err != nil {
		return nil, fmt.Errorf("ejecutar %q: %w", name, err)
	}
	return buf.Bytes(), nil
}

// HTML responde con la página name y el estado code. El estado se fija antes de escribir el cuerpo:
// fijarlo después no tiene efecto y toda respuesta saldría como 200.
func (r *Renderer) HTML(c *gin.Context, code int, name string, data any) {
	body, err := r.Execute(name, data)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusInternalServerError, "text/plain; charset=utf-8", []byte("Error interno al generar la página."))
		return
	}
	// Las páginas llevan contenido de documentos: que ningún caché las conserve.
	c.Header("Cache-Control", "no-store")
	c.Data(code, "text/html; charset=utf-8", body)
}
