package observability

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

func TestNewRejectsUnknownLevel(t *testing.T) {
	if _, err := New("no-existe", &bytes.Buffer{}); err == nil {
		t.Fatal("New(\"no-existe\") no falló, quería error")
	}
}

func TestNewHonorsLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, err := New("error", buf)
	if err != nil {
		t.Fatalf("New(\"error\") falló: %v", err)
	}

	logger.Info("silencio")
	if buf.Len() != 0 {
		t.Errorf("con nivel error se registró %q, esperaba nada", buf)
	}

	logger.Error("ruido")
	if buf.Len() == 0 {
		t.Error("con nivel error no se registró el error, esperaba sí")
	}
}

func TestWithDocumentIDAddsCorrelationField(t *testing.T) {
	const want = "0f8fad5b-d9cb-469f-a165-70867728950e"
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	WithDocumentID(logger, domain.DocumentID(want)).Info("procesando")

	if !bytes.Contains(buf.Bytes(), []byte(`"document_id":"`+want+`"`)) {
		t.Errorf("log = %s, quería document_id=%s", buf, want)
	}
}
