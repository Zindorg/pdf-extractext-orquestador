package render

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
)

//go:embed templates/*
var templatesFS embed.FS

// loadTemplates arma un conjunto de plantillas por página: layout + parciales + la página. Así cada
// página define su propio bloque "content" sin pisar el de las demás (en un conjunto único, la
// última definición gana y todas las páginas mostrarían el contenido de la misma).
func loadTemplates() (map[string]*template.Template, error) {
	entries, err := fs.ReadDir(templatesFS, "templates/pages")
	if err != nil {
		return nil, fmt.Errorf("leer las páginas: %w", err)
	}
	pages := make(map[string]*template.Template, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t, err := template.New(name).ParseFS(templatesFS,
			"templates/layouts/*.html",
			"templates/partials/*.html",
			"templates/pages/"+name,
		)
		if err != nil {
			return nil, fmt.Errorf("plantilla %s: %w", name, err)
		}
		pages[name] = t
	}
	return pages, nil
}
