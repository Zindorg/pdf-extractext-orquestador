// Package redis_streams publica eventos en el stream de ingesta (events.md). Es la única función
// de Redis del servicio: el Orquestador hace `XADD` y nada más —no lee, no consume, no mantiene el
// grupo (ADR-0001).
package redis_streams

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// schemaVersion es la versión del esquema del payload (events.md). Subirlo es una ruptura y exige
// avisar al equipo del Persistidor antes, no después.
const schemaVersion = 1

// Publisher publica eventos en el stream.
type Publisher struct {
	redis  xAdder
	stream string
}

// xAdder es la única primitiva de Redis que usa el servicio. El doble de test la reemplaza: este
// repo no levanta un Redis para probar al productor.
type xAdder interface {
	XAdd(ctx context.Context, stream, key string, payload []byte) error
	Close() error
}

// New arma el publicador apuntando a addr (por ejemplo `redis:6379`) en el stream dado.
func New(addr, stream string) *Publisher {
	return &Publisher{
		redis:  &redisXAdder{client: redis.NewClient(&redis.Options{Addr: addr})},
		stream: stream,
	}
}

// Publish implementa domain.EventPublisher.
func (p *Publisher) Publish(ctx context.Context, event domain.Event) error {
	key, payload, err := encode(event)
	if err != nil {
		return err
	}
	return p.redis.XAdd(ctx, p.stream, key, payload)
}

// Close libera la conexión de Redis para el apagado ordenado del proceso.
func (p *Publisher) Close() error {
	return p.redis.Close()
}

// encode arma el mensaje del stream. El JSON del evento va entero en el campo `payload` para
// conservar los tipos que events.md declara como número y booleano —un field de Redis es texto, y
// aplanarlo obligaría al consumidor a re-convertir cada campo—. El `document_id` se repite como
// field propio: es la clave del mensaje, la que permite identificarlo sin parsear el cuerpo.
func encode(event any) (string, []byte, error) {
	var key string
	var body any

	switch e := event.(type) {
	case domain.OriginalEvent:
		key = string(e.DocumentID)
		body = originalBody{
			SchemaVersion:    schemaVersion,
			EventType:        "original",
			DocumentID:       string(e.DocumentID),
			Checksum:         string(e.Checksum),
			ExtractedText:    e.ExtractedText,
			MimeType:         e.MimeType,
			SummaryRequested: e.SummaryRequested,
			SummaryStatus:    string(e.SummaryStatus),
			ExtractionTimeMS: e.ExtractionTimeMS,
			ExtractedAt:      e.ExtractedAt.UTC().Format(time.RFC3339),
		}
	case domain.SummaryResolvedEvent:
		key = string(e.DocumentID)
		body = summaryResolvedBody{
			SchemaVersion: schemaVersion,
			EventType:     "summary_resolved",
			DocumentID:    string(e.DocumentID),
			Checksum:      string(e.Checksum),
			Summary:       e.Summary,
			// Siempre RESOLVED: lo dice el nombre del evento, y el valor es constante (events.md).
			SummaryStatus: string(domain.SummaryResolved),
			SummaryTimeMS: e.SummaryTimeMS,
			ResolvedAt:    e.ResolvedAt.UTC().Format(time.RFC3339),
		}
	default:
		return "", nil, fmt.Errorf("evento no publicable: %T", event)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("serializar el evento: %w", err)
	}
	return key, payload, nil
}

type originalBody struct {
	SchemaVersion    int    `json:"schema_version"`
	EventType        string `json:"event_type"`
	DocumentID       string `json:"document_id"`
	Checksum         string `json:"checksum"`
	ExtractedText    string `json:"extracted_text"`
	MimeType         string `json:"mime_type"`
	SummaryRequested bool   `json:"summary_requested"`
	SummaryStatus    string `json:"summary_status"`
	ExtractionTimeMS int64  `json:"extraction_time_ms"`
	ExtractedAt      string `json:"extracted_at"`
}

type summaryResolvedBody struct {
	SchemaVersion int    `json:"schema_version"`
	EventType     string `json:"event_type"`
	DocumentID    string `json:"document_id"`
	Checksum      string `json:"checksum"`
	Summary       string `json:"summary"`
	SummaryStatus string `json:"summary_status"`
	SummaryTimeMS int64  `json:"summary_time_ms"`
	ResolvedAt    string `json:"resolved_at"`
}

// redisXAdder traduce la primitiva propia a la de go-redis.
type redisXAdder struct{ client *redis.Client }

func (r *redisXAdder) XAdd(ctx context.Context, stream, key string, payload []byte) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"document_id": key, "payload": string(payload)},
	}).Err()
}

func (r *redisXAdder) Close() error { return r.client.Close() }
