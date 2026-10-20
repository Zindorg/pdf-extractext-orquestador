// Command persister es un mock del Persistidor para el smoke local (compose.mocks.yml). No es parte
// del servicio y no se importa desde él.
//
// A diferencia de un stub de respuestas fijas, este mock **consume de verdad** el stream: lee con un
// consumer group (`docs/contracts/events.md`), aplica `original` y `summary_resolved` a un almacén en
// memoria, y sirve la API interna de §10 de ARCHITECTURE_REPOSITORIES.md. Así el smoke prueba el
// camino completo publicación -> persistencia -> lectura, no solo el proxy.
//
// Fidelidad cubierta: dedup por checksum activo (descarte silencioso), 409 de download/summary cuando
// el resumen está PENDING, 404 de documento inexistente, soft-delete + restore con conflicto de
// checksum, y el envelope de error `{code,message,details}`.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultAddr  = ":8083"
	groupName    = "documents-persister"
	consumerName = "mock-consumer"
)

// document es el registro en memoria. `summaryStatus` copia el valor del evento (no se deduce del
// par, ver events.md): es lo que distingue "no se pidió" de "se pidió y falló".
type document struct {
	DocumentID       string
	Checksum         string
	ExtractedText    string
	MimeType         string
	Summary          string
	SummaryRequested bool
	SummaryStatus    string
	ExtractionTimeMS int64
	SummaryTimeMS    int64
	ExtractedAt      string
	ResolvedAt       string
	Deleted          bool
}

func (d *document) processingTimeMS() int64 {
	total := d.ExtractionTimeMS
	if d.SummaryStatus == "RESOLVED" {
		total += d.SummaryTimeMS
	}
	return total
}

// store es el almacén en memoria, con los índices que el mock necesita para emular dedup y búsqueda
// por checksum. El candado cubre el ciclo de consumo y las lecturas HTTP a la vez.
type store struct {
	mu         sync.Mutex
	byID       map[string]*document
	byChecksum map[string]string // checksum activo -> document_id
}

func newStore() *store {
	return &store{byID: map[string]*document{}, byChecksum: map[string]string{}}
}

func main() {
	addr := env("MOCK_ADDR", defaultAddr)
	redisAddr := env("REDIS_ADDR", "redis:6379")
	stream := env("STREAM_NAME", "document-events")

	st := newStore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	go consume(ctx, rdb, stream, st)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", handleHealth(st))
	mux.HandleFunc("GET /api/v1/documents", handleList(st))
	mux.HandleFunc("GET /api/v1/documents/{document_id}", handleGet(st))
	mux.HandleFunc("GET /api/v1/documents/checksum/{checksum}", handleGetByChecksum(st))
	mux.HandleFunc("GET /api/v1/documents/{document_id}/download/original", handleDownloadOriginal(st))
	mux.HandleFunc("GET /api/v1/documents/{document_id}/download/summary", handleDownloadSummary(st))
	mux.HandleFunc("DELETE /api/v1/documents/{document_id}", handleDelete(st))
	mux.HandleFunc("POST /api/v1/documents/{document_id}/restore", handleRestore(st))

	log.Printf("persister mock escuchando en %s (stream=%s redis=%s)", addr, stream, redisAddr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("servidor del mock: %v", err)
	}
}

// consume lee el stream con un consumer group, imitando el contrato del consumidor: proceso =>
// XACK; duplicado legítimo => XACK + descarte silencioso. No implementa DLQ ni reintentos: el smoke
// no los ejercita y no hacen falta para probar el camino feliz.
func consume(ctx context.Context, rdb *redis.Client, stream string, st *store) {
	// Crear el grupo es idempotente; $ para no reprocesar lo que ya estaba antes de arrancar.
	if err := rdb.XGroupCreateMkStream(ctx, stream, groupName, "$").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		log.Printf("crear grupo: %v", err)
	}

	for {
		if ctx.Err() != nil {
			return
		}
		res, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: consumerName,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
		if err != nil && err != redis.Nil {
			log.Printf("leer stream: %v", err)
			time.Sleep(time.Second)
			continue
		}

		for _, s := range res {
			for _, msg := range s.Messages {
				applyMessage(st, msg)
				if err := rdb.XAck(ctx, stream, groupName, msg.ID).Err(); err != nil {
					log.Printf("ack %s: %v", msg.ID, err)
				}
			}
		}
	}
}

// applyMessage aplica un evento al almacén. Lee el JSON del field `payload` y el `document_id` del
// field propio, exactamente como el contrato del stream lo exige (delta #13).
func applyMessage(st *store, msg redis.XMessage) {
	raw, ok := msg.Values["payload"].(string)
	if !ok {
		log.Printf("mensaje %s sin field payload", msg.ID)
		return
	}

	var event struct {
		SchemaVersion    int    `json:"schema_version"`
		EventType        string `json:"event_type"`
		DocumentID       string `json:"document_id"`
		Checksum         string `json:"checksum"`
		ExtractedText    string `json:"extracted_text"`
		MimeType         string `json:"mime_type"`
		SummaryRequested bool   `json:"summary_requested"`
		SummaryStatus    string `json:"summary_status"`
		Summary          string `json:"summary"`
		ExtractionTimeMS int64  `json:"extraction_time_ms"`
		SummaryTimeMS    int64  `json:"summary_time_ms"`
		ExtractedAt      string `json:"extracted_at"`
		ResolvedAt       string `json:"resolved_at"`
	}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		log.Printf("payload ilegible en %s: %v", msg.ID, err)
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	switch event.EventType {
	case "original":
		if existing, busy := st.byChecksum[event.Checksum]; busy {
			log.Printf("original %s: checksum ya activo en %s, descarte", event.DocumentID, existing)
			return
		}
		st.byID[event.DocumentID] = &document{
			DocumentID:       event.DocumentID,
			Checksum:         event.Checksum,
			ExtractedText:    event.ExtractedText,
			MimeType:         event.MimeType,
			SummaryRequested: event.SummaryRequested,
			SummaryStatus:    event.SummaryStatus,
			ExtractionTimeMS: event.ExtractionTimeMS,
			ExtractedAt:      event.ExtractedAt,
		}
		st.byChecksum[event.Checksum] = event.DocumentID
		log.Printf("original %s ingresado (summary_status=%s)", event.DocumentID, event.SummaryStatus)

	case "summary_resolved":
		doc, ok := st.byID[event.DocumentID]
		if !ok {
			// El documento todavía no llegó: se descarta. El contrato garantiza el orden original
			// primero, así que en el camino feliz esto no pasa.
			log.Printf("summary_resolved %s sin original, descarte", event.DocumentID)
			return
		}
		if doc.SummaryStatus == "RESOLVED" {
			log.Printf("summary_resolved %s duplicado, descarte", event.DocumentID)
			return
		}
		doc.Summary = event.Summary
		doc.SummaryTimeMS = event.SummaryTimeMS
		doc.SummaryStatus = "RESOLVED"
		doc.ResolvedAt = event.ResolvedAt
		log.Printf("summary_resolved %s aplicado", event.DocumentID)

	default:
		log.Printf("event_type desconocido %q en %s", event.EventType, msg.ID)
	}
}

func handleHealth(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleGet(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("document_id")
		st.mu.Lock()
		doc, ok := st.byID[id]
		st.mu.Unlock()
		if !ok || doc.Deleted {
			notFound(w)
			return
		}
		writeJSON(w, http.StatusOK, documentBody(doc))
	}
}

func handleGetByChecksum(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checksum := r.PathValue("checksum")
		st.mu.Lock()
		doc, ok := st.byID[st.byChecksum[checksum]]
		st.mu.Unlock()
		if !ok || doc.Deleted {
			notFound(w)
			return
		}
		writeJSON(w, http.StatusOK, documentBody(doc))
	}
}

func handleDownloadOriginal(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		doc, ok := st.byID[r.PathValue("document_id")]
		st.mu.Unlock()
		if !ok || doc.Deleted {
			notFound(w)
			return
		}
		writeDownload(w, doc.DocumentID+".txt", doc.ExtractedText)
	}
}

func handleDownloadSummary(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		doc, ok := st.byID[r.PathValue("document_id")]
		st.mu.Unlock()
		if !ok || doc.Deleted {
			notFound(w)
			return
		}
		if doc.SummaryStatus != "RESOLVED" {
			// Aún PENDING: el Orquestador traduce este 409 a `summary_pending` (delta #12).
			writeJSON(w, http.StatusConflict, errorEnvelope{
				Code:    "summary_pending",
				Message: "el resumen aún no está disponible",
			})
			return
		}
		writeDownload(w, doc.DocumentID+"-summary.txt", doc.Summary)
	}
}

func handleDelete(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		doc, ok := st.byID[r.PathValue("document_id")]
		if !ok || doc.Deleted {
			st.mu.Unlock()
			notFound(w)
			return
		}
		doc.Deleted = true
		delete(st.byChecksum, doc.Checksum)
		st.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRestore(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		doc, ok := st.byID[r.PathValue("document_id")]
		if !ok {
			notFound(w)
			return
		}
		if !doc.Deleted {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if other, busy := st.byChecksum[doc.Checksum]; busy && other != doc.DocumentID {
			writeJSON(w, http.StatusConflict, errorEnvelope{
				Code:    "checksum_conflict",
				Message: "hay otro documento activo con el mismo checksum",
			})
			return
		}
		doc.Deleted = false
		st.byChecksum[doc.Checksum] = doc.DocumentID
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleList(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		items := make([]map[string]any, 0, len(st.byID))
		for _, doc := range st.byID {
			if doc.Deleted {
				continue
			}
			items = append(items, documentBody(doc))
		}
		st.mu.Unlock()

		sort.Slice(items, func(i, j int) bool {
			return fmt.Sprint(items[i]["document_id"]) < fmt.Sprint(items[j]["document_id"])
		})

		page := intQuery(r, "page", 1)
		pageSize := intQuery(r, "page_size", 20)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}
		total := len(items)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"items":     items[start:end],
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
	}
}

func documentBody(doc *document) map[string]any {
	body := map[string]any{
		"document_id":        doc.DocumentID,
		"checksum":           doc.Checksum,
		"extracted_text":     doc.ExtractedText,
		"mime_type":          doc.MimeType,
		"summary_requested":  doc.SummaryRequested,
		"summary_status":     doc.SummaryStatus,
		"extraction_time_ms": doc.ExtractionTimeMS,
		"processing_time_ms": doc.processingTimeMS(),
		"extracted_at":       doc.ExtractedAt,
		"deleted":            doc.Deleted,
	}
	if doc.SummaryStatus == "RESOLVED" {
		body["summary"] = doc.Summary
		body["summary_time_ms"] = doc.SummaryTimeMS
		body["resolved_at"] = doc.ResolvedAt
	}
	return body
}

type errorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, errorEnvelope{
		Code:    "document_not_found",
		Message: "el documento no existe",
	})
}

// writeDownload responde texto plano con Content-Disposition (RFC 5987), como §10.
func writeDownload(w http.ResponseWriter, filename, content string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", filename))
	_, _ = w.Write([]byte(content))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func intQuery(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
