package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	weberrors "github.com/Zindorg/pdf-extractext-orquestador/internal/web/errors"
)

func decodeJSONBody(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	return dec.Decode(v)
}

func (h *Handlers) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	filename := c.Query("filename")
	includeDeleted := c.Query("include_deleted") == "true"

	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 && pageSize <= 100 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	if status != "" {
		q.Set("status", status)
	}
	if filename != "" {
		q.Set("filename", filename)
	}
	if includeDeleted {
		q.Set("include_deleted", "true")
	}

	req := domain.PersisterRequest{
		Method:   "GET",
		Path:     "/api/v1/documents",
		RawQuery: q.Encode(),
	}

	resp, err := h.deps.Proxy.Forward(c.Request.Context(), req)
	if err != nil {
		h.renderer.HTML(c, http.StatusInternalServerError, "documents_list.html", gin.H{"Error": weberrors.UserMessage(err), "Items": []any{}})
		return
	}

	var out map[string]any
	_ = decodeJSONBody(resp.Body, &out)
	if out == nil {
		out = map[string]any{}
	}
	if _, ok := out["Items"]; !ok {
		if items, ok := out["items"]; ok {
			out["Items"] = items
		}
	}
	h.renderer.HTML(c, http.StatusOK, "documents_list.html", out)
}

func (h *Handlers) Detail(c *gin.Context) {
	id := c.Param("id")
	req := domain.PersisterRequest{
		Method: "GET",
		Path:   "/api/v1/documents/" + id,
	}
	resp, err := h.deps.Proxy.Forward(c.Request.Context(), req)
	if err != nil {
		var de *domain.Error
		if e, ok := err.(*domain.Error); ok {
			de = e
			if de.Code == "document_not_found" {
				h.renderer.HTML(c, http.StatusOK, "document_detail.html", gin.H{"Error": weberrors.UserMessage(err), "Processing": true, "Doc": map[string]any{"DocumentID": id, "SummaryStatus": "PENDING"}})
				return
			}
		}
		h.renderer.HTML(c, http.StatusNotFound, "document_detail.html", gin.H{"Error": weberrors.UserMessage(err)})
		return
	}
	var doc map[string]any
	_ = decodeJSONBody(resp.Body, &doc)
	if doc == nil {
		doc = map[string]any{}
	}
	doc["DocumentID"] = id
	h.renderer.HTML(c, http.StatusOK, "document_detail.html", gin.H{"Doc": doc})
}
