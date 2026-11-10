package observability

import (
	"bytes"
	"testing"
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
