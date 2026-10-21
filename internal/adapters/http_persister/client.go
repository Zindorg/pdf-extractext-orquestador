// Package http_persister implementa el proxy hacia el Persistidor: reenvía las rutas de lectura y
// de borrado sin reescribir el prefijo /api/v1 (ADR-0005).
package http_persister

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// Client es el cliente del proxy hacia el Persistidor.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New arma el cliente apuntando a baseURL (por ejemplo http://persister.internal) con el timeout
// dado. El timeout acota la espera: el proxy no debe retener la conexión del cliente sin límite.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// Do reenvía la petición al Persistidor. Path y query se concatenan sin tocar: la transparencia del
// proxy (delta #10) significa que lo que el cliente pidió es exactamente lo que recibe el otro
// servicio. Un `4xx` que trae un código del vocabulario público sale como error de dominio tipado;
// el resto (2xx, 5xx, transporte) sale como respuesta cruda o como error sin tipar.
func (c *Client) Do(ctx context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error) {
	target := c.baseURL + req.Path
	if req.RawQuery != "" {
		target += "?" + req.RawQuery
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, target, nil)
	if err != nil {
		return domain.PersisterResponse{}, fmt.Errorf("armar la petición al Persistidor: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return domain.PersisterResponse{}, fmt.Errorf("llamar al Persistidor: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.PersisterResponse{}, fmt.Errorf("leer la respuesta del Persistidor: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return domain.PersisterResponse{
			StatusCode: resp.StatusCode,
			Header:     map[string][]string(resp.Header),
			Body:       body,
		}, nil
	}

	return domain.PersisterResponse{}, translate(resp.StatusCode, body)
}

// translate convierte un error del Persistidor en uno de dominio. El código que declara el otro
// servicio manda cuando es del vocabulario público; si el cuerpo no se puede leer, el status decide
// solo donde el vocabulario no deja margen. Un 409 depende de la ruta (download/summary vs restore),
// así que sin código no se inventa: se devuelve sin tipar y la capa api responde 500.
func translate(status int, body []byte) error {
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		if known := domain.FromCode(envelope.Code); known != nil {
			return known
		}
	}

	switch status {
	case http.StatusBadRequest:
		return domain.ErrInvalidRequest
	case http.StatusNotFound:
		return domain.ErrDocumentNotFound
	}
	return fmt.Errorf("respuesta del Persistidor sin código reconocible (status %d)", status)
}

// FindActiveByChecksum consulta al Persistidor por checksum activo. Un 404 significa que no hay
// duplicado activo (no es error): el flujo normal de ingesta debe continuar.
func (c *Client) FindActiveByChecksum(ctx context.Context, checksum domain.Checksum) (domain.StoredDocumentSummary, bool, error) {
	target := c.baseURL + "/api/v1/documents/checksum/" + string(checksum)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return domain.StoredDocumentSummary{}, false, fmt.Errorf("armar la petición al Persistidor: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.StoredDocumentSummary{}, false, fmt.Errorf("llamar al Persistidor: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.StoredDocumentSummary{}, false, fmt.Errorf("leer la respuesta del Persistidor: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return domain.StoredDocumentSummary{}, false, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var payload struct {
			DocumentID    string `json:"document_id"`
			SummaryStatus string `json:"summary_status"`
			Summary       string `json:"summary"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return domain.StoredDocumentSummary{}, false, fmt.Errorf("decodificar documento por checksum: %w", err)
		}
		return domain.StoredDocumentSummary{
			DocumentID:    domain.DocumentID(payload.DocumentID),
			SummaryStatus: domain.SummaryStatus(payload.SummaryStatus),
			Summary:       payload.Summary,
		}, true, nil
	}

	return domain.StoredDocumentSummary{}, false, fmt.Errorf("respuesta del Persistidor sin código reconocible (status %d)", resp.StatusCode)
}
