// Package http_extractor implementa el cliente HTTP del Extractor contra su contrato en
// docs/contracts/workers.md.
package http_extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// mimeTypePDF es el único tipo que el Orquestador admite en el MVP (ADR-0008). El tipo viaja
// verificado por la firma del archivo, no por lo que declare el cliente (workers.md); cuando el MVP
// admita más formatos, el valor debe venir del dominio en lugar de vivir acá.
const mimeTypePDF = "application/pdf"

// maxRetries es la cantidad de reintentos ante un error de red, con espera creciente. Ante un error
// de validación no se reintenta: re-enviar el mismo payload va a fallar igual (workers.md).
const maxRetries = 2

// Client es el cliente HTTP del Extractor.
type Client struct {
	httpClient *http.Client
	endpoint   string
	backoff    func(attempt int) time.Duration
}

// New arma el cliente apuntando a baseURL (por ejemplo http://extractor.internal) con el timeout
// dado. El timeout es acotado: un worker colgado no debe poder retener el pedido del cliente.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		endpoint:   strings.TrimRight(baseURL, "/") + "/extract",
		backoff:    defaultBackoff,
	}
}

// defaultBackoff crece con el intento: 100ms, 200ms.
func defaultBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * 100 * time.Millisecond
}

// Extract implementa domain.Extractor. Un 422 no es un error: el motivo vuelve en la Extraction y
// lo clasifica la puerta 2 del caso de uso.
func (c *Client) Extract(ctx context.Context, documentID domain.DocumentID, in domain.IncomingDocument) (domain.Extraction, error) {
	for attempt := 0; ; attempt++ {
		extraction, retriable, err := c.extractOnce(ctx, documentID, in)
		if err == nil {
			return extraction, nil
		}
		if !retriable || attempt >= maxRetries {
			return domain.Extraction{}, err
		}
		if err := sleep(ctx, c.backoff(attempt+1)); err != nil {
			return domain.Extraction{}, fmt.Errorf("%w: %w", domain.ErrExtractorUnavailable, err)
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

func (c *Client) extractOnce(ctx context.Context, documentID domain.DocumentID, in domain.IncomingDocument) (domain.Extraction, bool, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writeFile(writer, "document", in.Filename, in.Bytes); err != nil {
		return domain.Extraction{}, false, unavailable("armar el campo document", err)
	}
	if err := writer.WriteField("document_id", string(documentID)); err != nil {
		return domain.Extraction{}, false, unavailable("armar el campo document_id", err)
	}
	if err := writer.WriteField("mime_type", mimeTypePDF); err != nil {
		return domain.Extraction{}, false, unavailable("armar el campo mime_type", err)
	}
	if err := writer.Close(); err != nil {
		return domain.Extraction{}, false, unavailable("cerrar el multipart", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		return domain.Extraction{}, false, unavailable("armar la petición", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Error de red (timeout, conexión rechazada, corte): es el único caso reintentable.
		return domain.Extraction{}, true, unavailable("llamar al Extractor", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeSuccess(resp)
	case http.StatusUnprocessableEntity:
		return decodeReject(resp)
	default:
		// Un estado no previsto es fallo del worker, no del documento: no se reintenta.
		return domain.Extraction{}, false, unavailable(fmt.Sprintf("status %d", resp.StatusCode), nil)
	}
}

func decodeSuccess(resp *http.Response) (domain.Extraction, bool, error) {
	var payload struct {
		ExtractedText    string    `json:"extracted_text"`
		ExtractionTimeMS int64     `json:"extraction_time_ms"`
		Warnings         []warning `json:"warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.Extraction{}, false, unavailable("leer la respuesta del Extractor", err)
	}

	return domain.Extraction{
		Text:     payload.ExtractedText,
		TimeMS:   payload.ExtractionTimeMS,
		Warnings: toWarnings(payload.Warnings),
		Reason:   domain.ReasonSuccess,
	}, false, nil
}

func decodeReject(resp *http.Response) (domain.Extraction, bool, error) {
	var payload struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		// El motivo no se pudo leer: se devuelve vacío y la puerta 2 lo trata como fallo del
		// Extractor, en vez de inventar una categoría.
		return domain.Extraction{}, false, nil
	}

	return domain.Extraction{Reason: domain.ExtractionReason(payload.Reason)}, false, nil
}

type warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func toWarnings(raw []warning) []domain.Warning {
	if len(raw) == 0 {
		return nil
	}
	out := make([]domain.Warning, 0, len(raw))
	for _, w := range raw {
		out = append(out, domain.Warning{Code: w.Code, Message: w.Message})
	}
	return out
}

func writeFile(writer *multipart.Writer, field, filename string, content []byte) error {
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	return nil
}

// unavailable envuelve la causa en ErrExtractorUnavailable, que es lo que el caso de uso devuelve
// tal cual a la capa api (ADR-0008). El detalle viaja envuelto para diagnóstico; el mensaje que ve
// el cliente lo decide la capa api, nunca este paquete.
func unavailable(what string, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %s: %w", domain.ErrExtractorUnavailable, what, err)
	}
	return fmt.Errorf("%w: %s", domain.ErrExtractorUnavailable, what)
}
