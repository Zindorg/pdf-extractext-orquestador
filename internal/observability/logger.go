// Package observability concentra la observabilidad del servicio: logging estructurado con
// correlación por document_id (ADR-0004).
package observability

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// levels mapea el nombre del nivel a su valor. El servicio no reconoce otros.
var levels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// New devuelve un logger JSON estructurado en w. Un nivel desconocido es un error de configuración:
// conviene fallar al arrancar que quedar a ciegas en producción.
func New(level string, w io.Writer) (*slog.Logger, error) {
	value, ok := levels[level]
	if !ok {
		return nil, fmt.Errorf("nivel de log desconocido: %q", level)
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: value})), nil
}

// WithDocumentID devuelve una copia del logger con el identificador del documento como correlación.
// Es el único correlador del sistema (ADR-0004).
func WithDocumentID(logger *slog.Logger, id domain.DocumentID) *slog.Logger {
	return logger.With("document_id", id)
}
