// Command extractor es un mock del Extractor (docs/contracts/workers.md) para el smoke local
// (compose.mocks.yml). No es parte del servicio y no se importa desde él: es un vecino de prueba.
//
// El comportamiento por defecto imita un PDF sano: devuelve 200 con texto extraído. El caso de
// prueba se elige por el nombre del archivo, para poder disparar los motivos de rechazo sin tocar
// el Orquestador:
//
//	contrato.pdf       -> 200 success
//	vacio.pdf          -> 200 con texto vacío (la puerta 2 del Orquestador lo rechaza)
//	cifrado.pdf        -> 422 encrypted_pdf
//	roto.pdf           -> 422 corrupt
//	escaneado.pdf      -> 422 unsupported_features
//	lento.pdf          -> demora EXTRACTOR_MOCK_DELAY_MS antes de responder (para timeouts)
//	cualquier otro     -> 200 success
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

// textByFilename da un texto estable por caso de prueba, para que el checksum publicado sea
// predecible y el smoke pueda afirmar sobre él.
var textByFilename = map[string]string{
	"contrato.pdf":        "texto extraído del contrato de prueba",
	"lento.pdf":           "texto extraído con demora",
	"demasiado-largo.pdf": "texto con [demasiado-largo] para forzar document_too_long",
}

// rejectByFilename mapea un archivo al motivo del enum de extracción (workers.md). El Orquestador
// traduce el motivo a 422 y no publica nada.
var rejectByFilename = map[string]string{
	"cifrado.pdf":   "encrypted_pdf",
	"roto.pdf":      "corrupt",
	"escaneado.pdf": "unsupported_features",
}

func main() {
	addr := os.Getenv("MOCK_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /extract", handleExtract)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	log.Printf("extractor mock escuchando en %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("servidor del mock: %v", err)
	}
}

func handleExtract(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"reason":  "invalid_request",
			"message": "multipart inválido",
		})
		return
	}

	filename := filenameOf(r)
	documentID := r.FormValue("document_id")
	log.Printf("extract: file=%q document_id=%q mime=%q", filename, documentID, r.FormValue("mime_type"))

	if delay := delayFor(filename); delay > 0 {
		time.Sleep(delay)
	}

	if reason, ok := rejectByFilename[filename]; ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"reason":  reason,
			"message": "rechazo simulado para " + filename,
		})
		return
	}

	text := textByFilename[filename]
	if strings.HasPrefix(filename, "vacio") {
		text = ""
	}
	if text == "" && !strings.HasPrefix(filename, "vacio") {
		text = "texto extraído de prueba por defecto"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"extracted_text":     text,
		"extraction_time_ms": extractionTimeMS(filename),
		"warnings":           warningsFor(filename),
	})
}

// filenameOf saca el nombre del archivo del campo `document` sin cargarlo en memoria de nuevo: solo
// se lee la cabecera de la parte.
func filenameOf(r *http.Request) string {
	if r.MultipartForm == nil {
		return ""
	}
	files := r.MultipartForm.File["document"]
	if len(files) == 0 {
		return ""
	}
	return sanitize(files[0].Filename)
}

// sanitize descarta cualquier ruta que traiga el cliente: el mock solo necesita el nombre base.
func sanitize(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		return name[i+1:]
	}
	return name
}

// delayFor lee EXTRACTOR_MOCK_DELAY_MS cuando el archivo es `lento*`; así el smoke puede probar el
// timeout del cliente sin un servidor colgado de verdad.
func delayFor(filename string) time.Duration {
	if !strings.HasPrefix(filename, "lento") {
		return 0
	}
	return envDurationMS("EXTRACTOR_MOCK_DELAY_MS")
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

// extractionTimeMS es un valor fijo y no el tiempo real: el mock no mide nada y un número variable
// haría inestable la aserción del smoke.
func extractionTimeMS(string) int64 { return 7 }

func warningsFor(filename string) []map[string]string {
	if strings.HasPrefix(filename, "parcial") {
		return []map[string]string{{"code": "partial", "message": "texto extraído parcialmente"}}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
