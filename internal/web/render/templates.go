package render

import (
	"embed"
	"html/template"
)

//go:embed templates/*
var templatesFS embed.FS

func loadTemplates() (*template.Template, error) {
	return template.ParseFS(templatesFS, "templates/layouts/*.html", "templates/pages/*.html", "templates/partials/*.html")
}
