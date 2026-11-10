package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	weberrors "github.com/Zindorg/pdf-extractext-orquestador/internal/web/errors"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

// List muestra el listado paginado con filtros.
func (h *Handlers) List(c *gin.Context) {
	q := view.ParseListQuery(c.Query("page"), c.Query("page_size"), c.Query("status"),
		c.Query("filename"), c.Query("include_deleted"))
	base := flash(c, "documents")

	resp, err := h.deps.Proxy.Forward(c.Request.Context(), domain.PersisterRequest{
		Method:   http.MethodGet,
		Path:     "/api/v1/documents",
		RawQuery: q.PersisterQuery(),
	})
	if err != nil {
		status, msg := http.StatusBadGateway, "No pudimos consultar el servicio de documentos. Probá de nuevo en unos segundos."
		var de *domain.Error
		if errors.As(err, &de) {
			status, msg = weberrors.StatusCode(err), weberrors.UserMessage(err)
		} else {
			h.log().Error("fallo al listar documentos", "error", err)
		}
		l := view.BuildList(q, view.ListPayload{})
		l.Base = view.Base{Nav: "documents", Error: msg}
		h.renderer.HTML(c, status, "documents_list.html", l)
		return
	}

	var payload view.ListPayload
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		h.log().Error("respuesta ilegible del Persistidor al listar", "error", err)
		l := view.BuildList(q, view.ListPayload{})
		l.Base = view.Base{Nav: "documents", Error: "La respuesta del servicio de documentos no es válida."}
		h.renderer.HTML(c, http.StatusBadGateway, "documents_list.html", l)
		return
	}
	l := view.BuildList(q, payload)
	l.Base = base
	h.renderer.HTML(c, http.StatusOK, "documents_list.html", l)
}

// Detail muestra un documento. Si se acaba de subir y el Persistidor todavía no lo asentó, muestra un
// estado de espera que se recarga solo; si el resumen está en curso, también.
func (h *Handlers) Detail(c *gin.Context) {
	id := c.Param("id")
	kind, n := view.ParseWait(c.Query("wait"), c.Query("n"))
	base := flash(c, "documents")

	doc, err := h.fetchDocument(c.Request.Context(), id)
	if err != nil {
		// Contrato (openapi.yaml): tras el 202, el 404 es transitorio durante 60 s; pasada la ventana es
		// terminal. La ventana la fija el contador `n` de la URL, que el servidor acota.
		if errCode(err) == "document_not_found" && kind == view.WaitDoc && n < view.MaxWaitAttempts {
			h.renderer.HTML(c, http.StatusOK, "document_detail.html", view.Detail{
				Base: base,
				ID:   id,
				Wait: view.NewWait(id, kind, n),
			})
			return
		}
		h.documentError(c, err, id)
		return
	}

	d := view.BuildDetail(doc, id)
	d.Base = base
	switch {
	case kind == view.WaitSummary && d.Status.Code != view.StatusResolved:
		d.Wait = view.NewWait(id, kind, n)
	case d.Status.Code == view.StatusPending:
		if kind != view.WaitPending {
			n = 0
		}
		d.Wait = view.NewWait(id, view.WaitPending, n)
	}
	h.renderer.HTML(c, http.StatusOK, "document_detail.html", d)
}

// FullText muestra el texto extraído completo.
func (h *Handlers) FullText(c *gin.Context) {
	id := c.Param("id")
	doc, err := h.fetchDocument(c.Request.Context(), id)
	if err != nil {
		h.documentError(c, err, id)
		return
	}
	t := view.BuildFullText(doc, id)
	t.Base = view.Base{Nav: "documents"}
	h.renderer.HTML(c, http.StatusOK, "document_text.html", t)
}
