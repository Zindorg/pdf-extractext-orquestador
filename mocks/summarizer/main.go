// Command summarizer es un mock del Resumidor (docs/contracts/workers.md) para el smoke local
// (compose.mocks.yml). No es parte del servicio y no se importa desde él.
//
// El comportamiento por defecto imita un resumen sano: 200 con `summary`. El caso de prueba se elige
// por el contenido del texto extraído, para disparar el rechazo sin tocar al Orquestador:
//
//	texto que contiene "[demasiado-largo]" -> 422 document_too_long (→ 502 en la API pública)
//	texto que contiene "[resumen-vacio]"   -> 200 con summary vacío (falla el Orquestador)
//	cualquier otro texto                   -> 200 summary derivado del texto
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAddr = ":8080"

// markTooLong y markEmpty son centinelas en el texto que el smoke manda: permiten elegir el caso sin
// un canal aparte, porque el Orquestador es el único cliente y controla el texto.
const (
	markTooLong = "[demasiado-largo]"
	markEmpty   = "[resumen-vacio]"
)

func main() {
	addr := os.Getenv("MOCK_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /summarize", handleSummarize)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	log.Printf("summarizer mock escuchando en %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("servidor del mock: %v", err)
	}
}

func handleSummarize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocumentID    string `json:"document_id"`
		ExtractedText string `json:"extracted_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"reason":  "invalid_request",
			"message": "JSON inválido",
		})
		return
	}

	log.Printf("summarize: document_id=%q text_len=%d", req.DocumentID, len(req.ExtractedText))

	if delay := envDurationMS("SUMMARIZER_MOCK_DELAY_MS"); delay > 0 {
		time.Sleep(delay)
	}

	if strings.Contains(req.ExtractedText, markTooLong) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"reason":  "document_too_long",
			"message": "el texto supera la ventana de contexto",
		})
		return
	}

	summary := "resumen de prueba"
	if strings.Contains(req.ExtractedText, markEmpty) {
		summary = ""
	}

	// summary_time_ms es fijo a propósito: el mock no mide y un valor variable haría inestable la
	// aserción del smoke.
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":         summary,
		"summary_time_ms": 11,
	})
}

func envDurationMS(name string) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
