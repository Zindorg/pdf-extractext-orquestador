package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	weberrors "github.com/Zindorg/pdf-extractext-orquestador/internal/web/errors"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/render"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

// Los casos de uso se piden como interfaces mínimas: los de application las cumplen y los tests
// inyectan dobles sin levantar el sistema.

// Extractor admite un documento (extracción, resumen opcional y publicación).
type Extractor interface {
	Execute(ctx context.Context, in domain.IncomingDocument) (domain.IngestResult, error)
}

// Summarizer pide el resumen de un documento que ya existe.
type Summarizer interface {
	Execute(ctx context.Context, id domain.DocumentID) (domain.SummaryRequestResult, error)
}

// Forwarder reenvía una petición al Persistidor.
type Forwarder interface {
	Forward(ctx context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error)
}

// Deps son las dependencias de los handlers web.
type Deps struct {
	Extract       Extractor
	Summary       Summarizer
	Proxy         Forwarder
	PublicBaseURL string
	Logger        *slog.Logger
}

// Handlers agrupa los handlers de la interfaz web.
type Handlers struct {
	deps     Deps
	renderer *render.Renderer
}

// New arma los handlers.
func New(d Deps, r *render.Renderer) *Handlers {
	return &Handlers{deps: d, renderer: r}
}

func (h *Handlers) log() *slog.Logger {
	if h.deps.Logger != nil {
		return h.deps.Logger
	}
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// errCode devuelve el código de un error de dominio, o "internal" si no lo es.
func errCode(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return "internal"
}

// redirect responde con 303 See Other: tras un POST, el navegador debe seguir con un GET.
func redirect(c *gin.Context, location string) {
	c.Redirect(http.StatusSeeOther, location)
}

// flash lee los avisos que viajan en la query tras una redirección. Solo se muestran textos propios:
// el código de aviso se traduce y el de error pasa por el catálogo de mensajes, así que nada de lo
// que llega por la URL se refleja tal cual.
func flash(c *gin.Context, nav string) view.Base {
	b := view.Base{Nav: nav, Notice: view.NoticeText(c.Query("notice"))}
	if code := c.Query("error"); code != "" {
		b.Error = weberrors.UserMessage(&domain.Error{Code: code, Message: "La solicitud no es válida."})
	}
	return b
}

// page responde con una página de error propia, en lugar de un cuerpo vacío.
func (h *Handlers) page(c *gin.Context, status int, title, message, backURL, backLabel string) {
	h.renderer.HTML(c, status, "error.html", view.ErrorPage{
		Base:      view.Base{},
		Status:    status,
		Title:     title,
		Message:   message,
		BackURL:   backURL,
		BackLabel: backLabel,
	})
}

// documentError traduce un error al consultar o descargar un documento.
func (h *Handlers) documentError(c *gin.Context, err error, id string) {
	var de *domain.Error
	if !errors.As(err, &de) {
		h.log().Error("fallo al consultar el Persistidor", "document_id", id, "error", err)
		h.page(c, http.StatusBadGateway, "No pudimos consultar el documento",
			"El servicio de documentos no respondió. Probá de nuevo en unos segundos.", "/documents", "Volver al listado")
		return
	}
	switch de.Code {
	case "document_not_found":
		h.page(c, http.StatusNotFound, "No encontramos el documento",
			"No existe, fue borrado, o todavía no terminó de guardarse. Si lo acabás de subir, probá de nuevo en unos segundos.",
			"/documents", "Volver al listado")
	case "summary_not_available", "summary_pending":
		h.page(c, weberrors.StatusCode(err), "Resumen no disponible", weberrors.UserMessage(err),
			"/documents/"+id, "Volver al documento")
	default:
		h.page(c, weberrors.StatusCode(err), "No se pudo completar la acción", weberrors.UserMessage(err),
			"/documents/"+id, "Volver al documento")
	}
}

// fetchDocument lee un documento del Persistidor.
func (h *Handlers) fetchDocument(ctx context.Context, id string) (view.PersistedDoc, error) {
	var doc view.PersistedDoc
	resp, err := h.deps.Proxy.Forward(ctx, domain.PersisterRequest{Method: http.MethodGet, Path: "/api/v1/documents/" + id})
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return doc, fmt.Errorf("respuesta ilegible del Persistidor: %w", err)
	}
	return doc, nil
}

// NotFound responde las rutas desconocidas: HTML para el navegador y el 404 de siempre para /api/.
func (h *Handlers) NotFound(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.String(http.StatusNotFound, "404 page not found")
		return
	}
	h.page(c, http.StatusNotFound, "Página no encontrada", "Esa dirección no existe.", "/", "Ir al inicio")
}
