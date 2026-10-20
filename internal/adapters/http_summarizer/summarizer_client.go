// Package http_summarizer implementa el cliente HTTP del Resumidor contra su contrato en
// docs/contracts/workers.md.
package http_summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// maxRetries es la cantidad de reintentos ante un error de red, con espera creciente. Ante un 4xx no
// se reintenta: re-enviar el mismo texto va a fallar igual (workers.md).
const maxRetries = 2

// Client es el cliente HTTP del Resumidor.
type Client struct {
	httpClient *http.Client
	endpoint   string
	backoff    func(attempt int) time.Duration
}

// New arma el cliente apuntando a baseURL (por ejemplo http://summarizer.internal) con el timeout
// dado. El timeout es holgado por diseño: el modelo local es serializado y tarda minutos (workers.md).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		endpoint:   strings.TrimRight(baseURL, "/") + "/summarize",
		backoff:    defaultBackoff,
	}
}

// defaultBackoff crece con el intento: 100ms, 200ms.
func defaultBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * 100 * time.Millisecond
}

// Summarize implementa domain.Summarizer. Un 422 (hoy solo `document_too_long`) es ErrSummaryFailed:
// en la API pública es 502, no un error del documento, y por la regla de publicar solo al final no se
// ingirió nada (workers.md).
func (c *Client) Summarize(ctx context.Context, documentID domain.DocumentID, text string) (domain.Summary, error) {
	for attempt := 0; ; attempt++ {
		summary, retriable, err := c.summarizeOnce(ctx, documentID, text)
		if err == nil {
			return summary, nil
		}
		if !retriable || attempt >= maxRetries {
			return domain.Summary{}, err
		}
		if err := sleep(ctx, c.backoff(attempt+1)); err != nil {
			return domain.Summary{}, summaryFailed("esperar el reintento", err)
		}
	}
}

// sleep espera d o cancela con el contexto. Un d menor o igual a cero no espera.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func (c *Client) summarizeOnce(ctx context.Context, documentID domain.DocumentID, text string) (domain.Summary, bool, error) {
	payload, err := json.Marshal(map[string]string{
		"document_id":    string(documentID),
		"extracted_text": text,
	})
	if err != nil {
		return domain.Summary{}, false, summaryFailed("armar la petición", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return domain.Summary{}, false, summaryFailed("armar la petición", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Error de red (timeout, conexión rechazada, corte): es el único caso reintentable.
		return domain.Summary{}, true, summaryFailed("llamar al Resumidor", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeSuccess(resp)
	case http.StatusUnprocessableEntity:
		return domain.Summary{}, false, summaryFailed("el Resumidor rechazó el texto", nil)
	default:
		// Un estado no previsto es fallo del worker: no se reintenta.
		return domain.Summary{}, false, summaryFailed(fmt.Sprintf("status %d", resp.StatusCode), nil)
	}
}

func decodeSuccess(resp *http.Response) (domain.Summary, bool, error) {
	var payload struct {
		Summary       string `json:"summary"`
		SummaryTimeMS int64  `json:"summary_time_ms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.Summary{}, false, summaryFailed("leer la respuesta del Resumidor", err)
	}

	return domain.Summary{Text: payload.Summary, TimeMS: payload.SummaryTimeMS}, false, nil
}

// summaryFailed envuelve la causa en ErrSummaryFailed, que es lo que el caso de uso devuelve tal cual
// a la capa api (ADR-0008). El detalle viaja envuelto para diagnóstico; el mensaje que ve el cliente
// lo decide la capa api, nunca este paquete.
func summaryFailed(what string, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %s: %w", domain.ErrSummaryFailed, what, err)
	}
	return fmt.Errorf("%w: %s", domain.ErrSummaryFailed, what)
}
