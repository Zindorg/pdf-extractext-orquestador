package domain

import (
	"path/filepath"
	"strings"
)

// maxFilenameRunes acota el nombre declarado por el cliente: viaja en el evento y se muestra en la web.
const maxFilenameRunes = 255

// CleanFilename deja solo el nombre del archivo que declaró el cliente, sin la ruta que algunos
// navegadores antiguos envían, y con un largo razonable. El archivo no se guarda: el nombre es un dato
// que se muestra, no una ruta que se use.
func CleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" {
		return ""
	}
	if r := []rune(name); len(r) > maxFilenameRunes {
		name = string(r[:maxFilenameRunes])
	}
	return name
}
