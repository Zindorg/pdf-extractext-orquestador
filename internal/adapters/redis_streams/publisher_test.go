package redis_streams

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// fakeAdder captura el XADD sin tocar Redis: este servicio solo usa esa primitiva (ADR-0001).
type fakeAdder struct {
	calls   int
	stream  string
	key     string
	payload []byte
	err     error
}

func (f *fakeAdder) XAdd(_ context.Context, stream, key string, payload []byte) error {
	f.calls++
	f.stream, f.key, f.payload = stream, key, payload
	return f.err
}

func (f *fakeAdder) Close() error { return nil }

const testStream = "document-events"

func newPublisher(adder *fakeAdder) *Publisher {
	return &Publisher{redis: adder, stream: testStream}
}

func decodePayload(t *testing.T, adder *fakeAdder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(adder.payload, &payload); err != nil {
		t.Fatalf("payload ilegible: %v (%s)", err, adder.payload)
	}
	return payload
}

func requireFields(t *testing.T, payload map[string]any, fields ...string) {
	t.Helper()

	for _, field := range fields {
		if _, ok := payload[field]; !ok {
			t.Errorf("payload sin el campo obligatorio %q: %s", field, payload)
		}
	}
}

const (
	testDocumentID = domain.DocumentID("0f8fad5b-d9cb-469f-a165-70867728950e")
	testChecksum   = domain.Checksum("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
)

var extractedAt = time.Date(2026, time.September, 30, 14, 22, 5, 0, time.UTC)

func TestPublishOriginalMessage(t *testing.T) {
	adder := &fakeAdder{}
	publisher := newPublisher(adder)

	event := domain.OriginalEvent{
		DocumentID:       testDocumentID,
		Checksum:         testChecksum,
		ExtractedText:    "texto plano del documento",
		MimeType:         "application/pdf",
		SummaryRequested: true,
		SummaryStatus:    domain.SummaryResolved,
		ExtractionTimeMS: 1240,
		ExtractedAt:      extractedAt,
	}
	if err := publisher.Publish(t.Context(), event); err != nil {
		t.Fatalf("Publish() falló: %v", err)
	}

	if adder.calls != 1 {
		t.Fatalf("XADD = %d, quería 1", adder.calls)
	}
	if adder.stream != testStream {
		t.Errorf("stream = %q, quería %q", adder.stream, testStream)
	}
	if adder.key != string(testDocumentID) {
		t.Errorf("clave del mensaje = %q, quería %q", adder.key, testDocumentID)
	}

	payload := decodePayload(t, adder)
	requireFields(t, payload,
		"schema_version", "event_type", "document_id", "checksum", "extracted_text",
		"mime_type", "summary_requested", "summary_status", "extraction_time_ms", "extracted_at")

	if got := payload["schema_version"]; got != float64(1) {
		t.Errorf("schema_version = %v, quería 1", got)
	}
	if got := payload["event_type"]; got != "original" {
		t.Errorf("event_type = %v, quería original", got)
	}
	if got := payload["document_id"]; got != string(testDocumentID) {
		t.Errorf("document_id = %v, quería %q", got, testDocumentID)
	}
	if got := payload["checksum"]; got != string(testChecksum) {
		t.Errorf("checksum = %v, quería %q", got, testChecksum)
	}
	if got := payload["extracted_text"]; got != "texto plano del documento" {
		t.Errorf("extracted_text = %v", got)
	}
	if got := payload["mime_type"]; got != "application/pdf" {
		t.Errorf("mime_type = %v", got)
	}
	if got := payload["summary_requested"]; got != true {
		t.Errorf("summary_requested = %v, quería true", got)
	}
	if got := payload["summary_status"]; got != "RESOLVED" {
		t.Errorf("summary_status = %v, quería RESOLVED", got)
	}
	if got := payload["extraction_time_ms"]; got != float64(1240) {
		t.Errorf("extraction_time_ms = %v, quería 1240", got)
	}
	if got := payload["extracted_at"]; got != "2026-09-30T14:22:05Z" {
		t.Errorf("extracted_at = %v, quería 2026-09-30T14:22:05Z", got)
	}
}

func TestPublishOriginalWithoutSummary(t *testing.T) {
	adder := &fakeAdder{}
	publisher := newPublisher(adder)

	event := domain.OriginalEvent{
		DocumentID:       testDocumentID,
		Checksum:         testChecksum,
		ExtractedText:    "texto",
		MimeType:         "application/pdf",
		SummaryRequested: false,
		SummaryStatus:    domain.SummaryNotRequested,
		ExtractionTimeMS: 10,
		ExtractedAt:      extractedAt,
	}
	if err := publisher.Publish(t.Context(), event); err != nil {
		t.Fatalf("Publish() falló: %v", err)
	}

	payload := decodePayload(t, adder)
	if got := payload["summary_requested"]; got != false {
		t.Errorf("summary_requested = %v, quería false", got)
	}
	if got := payload["summary_status"]; got != "NOT_REQUESTED" {
		t.Errorf("summary_status = %v, quería NOT_REQUESTED", got)
	}
}

func TestPublishSummaryResolvedMessage(t *testing.T) {
	adder := &fakeAdder{}
	publisher := newPublisher(adder)

	event := domain.SummaryResolvedEvent{
		DocumentID:    testDocumentID,
		Checksum:      testChecksum,
		Summary:       "síntesis en lenguaje natural",
		SummaryTimeMS: 8300,
		ResolvedAt:    extractedAt,
	}
	if err := publisher.Publish(t.Context(), event); err != nil {
		t.Fatalf("Publish() falló: %v", err)
	}

	if adder.key != string(testDocumentID) {
		t.Errorf("clave del mensaje = %q, quería %q", adder.key, testDocumentID)
	}

	payload := decodePayload(t, adder)
	requireFields(t, payload,
		"schema_version", "event_type", "document_id", "checksum", "summary",
		"summary_status", "summary_time_ms", "resolved_at")

	if got := payload["event_type"]; got != "summary_resolved" {
		t.Errorf("event_type = %v, quería summary_resolved", got)
	}
	if got := payload["summary_status"]; got != "RESOLVED" {
		t.Errorf("summary_status = %v, quería la constante RESOLVED", got)
	}
	if got := payload["summary"]; got != "síntesis en lenguaje natural" {
		t.Errorf("summary = %v", got)
	}
	if got := payload["summary_time_ms"]; got != float64(8300) {
		t.Errorf("summary_time_ms = %v, quería 8300", got)
	}
	if got := payload["resolved_at"]; got != "2026-09-30T14:22:05Z" {
		t.Errorf("resolved_at = %v, quería 2026-09-30T14:22:05Z", got)
	}
}

// `processing_time_ms` no viaja en ningún evento: cada uno lleva solo su pata (events.md).
func TestPublishDoesNotCarryTotalProcessingTime(t *testing.T) {
	adder := &fakeAdder{}
	publisher := newPublisher(adder)

	original := domain.OriginalEvent{
		DocumentID: testDocumentID, Checksum: testChecksum, ExtractedText: "texto",
		MimeType: "application/pdf", SummaryStatus: domain.SummaryNotRequested, ExtractedAt: extractedAt,
	}
	if err := publisher.Publish(t.Context(), original); err != nil {
		t.Fatalf("Publish(original) falló: %v", err)
	}
	if _, ok := decodePayload(t, adder)["processing_time_ms"]; ok {
		t.Error("original lleva processing_time_ms, que no existe en events.md")
	}

	summary := domain.SummaryResolvedEvent{
		DocumentID: testDocumentID, Checksum: testChecksum, Summary: "resumen",
		SummaryTimeMS: 1, ResolvedAt: extractedAt,
	}
	if err := publisher.Publish(t.Context(), summary); err != nil {
		t.Fatalf("Publish(summary_resolved) falló: %v", err)
	}
	if _, ok := decodePayload(t, adder)["processing_time_ms"]; ok {
		t.Error("summary_resolved lleva processing_time_ms, que no existe en events.md")
	}
}

// Un evento que el dominio agregue sin que este paquete lo conozca no se publica a medias.
func TestEncodeRejectsUnknownEvent(t *testing.T) {
	key, payload, err := encode(unknownEvent{})
	if err == nil {
		t.Fatal("encode() con un evento desconocido no falló")
	}
	if key != "" || payload != nil {
		t.Errorf("encode() = (%q, %s), quería nada publicado", key, payload)
	}
}

func TestPublishPropagatesStreamError(t *testing.T) {
	adder := &fakeAdder{err: errors.New("redis no responde")}

	err := newPublisher(adder).Publish(t.Context(), domain.OriginalEvent{
		DocumentID: testDocumentID, Checksum: testChecksum, ExtractedText: "texto",
		MimeType: "application/pdf", ExtractedAt: extractedAt,
	})
	if err == nil {
		t.Fatal("Publish() no falló con Redis caído")
	}
}

type unknownEvent struct{}

func (unknownEvent) isEvent() {}
