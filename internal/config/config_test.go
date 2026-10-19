package config

import (
	"strings"
	"testing"
	"time"
)

// envMap devuelve una getenv cerrada sobre el mapa, para no tocar el entorno del proceso en los tests.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// validEnv es el entorno mínimo que satisface todas las variables requeridas.
func validEnv(overrides map[string]string) map[string]string {
	m := map[string]string{
		"PUBLIC_BASE_URL": "https://api.example.com",
		"EXTRACTOR_URL":   "http://extractor.internal",
		"SUMMARIZER_URL":  "http://summarizer.internal",
		"PERSISTER_URL":   "http://persister.internal",
		"REDIS_ADDR":      "redis:6379",
	}
	for k, v := range overrides {
		m[k] = v
	}
	return m
}

func TestLoadFailsFastWhenRequiredIsMissing(t *testing.T) {
	required := []string{"PUBLIC_BASE_URL", "EXTRACTOR_URL", "SUMMARIZER_URL", "PERSISTER_URL", "REDIS_ADDR"}

	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			env := validEnv(nil)
			delete(env, name)

			if _, err := load(envMap(env)); err == nil {
				t.Fatalf("load() sin %s no falló, quería error de configuración", name)
			} else if !strings.Contains(err.Error(), name) {
				t.Fatalf("error = %q, quería que mencione %s", err, name)
			}
		})
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := load(envMap(validEnv(nil)))
	if err != nil {
		t.Fatalf("load() con entorno válido falló: %v", err)
	}

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, quería 8080", cfg.HTTPPort)
	}
	if cfg.MaxDocumentBytes != 10<<20 {
		t.Errorf("MaxDocumentBytes = %d, quería %d (10 MiB)", cfg.MaxDocumentBytes, 10<<20)
	}
	if cfg.StreamName != "document-events" {
		t.Errorf("StreamName = %q, quería document-events", cfg.StreamName)
	}
	if cfg.ExtractorTimeout != 30*time.Second {
		t.Errorf("ExtractorTimeout = %v, quería 30s", cfg.ExtractorTimeout)
	}
	if cfg.SummarizerTimeout != 120*time.Second {
		t.Errorf("SummarizerTimeout = %v, quería 120s (holgado por diseño)", cfg.SummarizerTimeout)
	}
	if cfg.ExtractMaxInFlight != 16 {
		t.Errorf("ExtractMaxInFlight = %d, quería 16", cfg.ExtractMaxInFlight)
	}
	if cfg.SummaryMaxInFlight != 4 {
		t.Errorf("SummaryMaxInFlight = %d, quería 4 (semáforo angosto)", cfg.SummaryMaxInFlight)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, quería info", cfg.LogLevel)
	}
}

func TestLoadOverridesDefaultsFromEnv(t *testing.T) {
	cfg, err := load(envMap(validEnv(map[string]string{
		"HTTP_PORT":             "9090",
		"MAX_DOCUMENT_BYTES":    "2048",
		"STREAM_NAME":           "otros-eventos",
		"EXTRACTOR_TIMEOUT":     "5s",
		"SUMMARIZER_TIMEOUT":    "7s",
		"EXTRACT_MAX_IN_FLIGHT": "3",
		"SUMMARY_MAX_IN_FLIGHT": "1",
		"LOG_LEVEL":             "debug",
	})))
	if err != nil {
		t.Fatalf("load() falló: %v", err)
	}

	if cfg.HTTPPort != 9090 {
		t.Errorf("HTTPPort = %d, quería 9090", cfg.HTTPPort)
	}
	if cfg.MaxDocumentBytes != 2048 {
		t.Errorf("MaxDocumentBytes = %d, quería 2048", cfg.MaxDocumentBytes)
	}
	if cfg.StreamName != "otros-eventos" {
		t.Errorf("StreamName = %q, quería otros-eventos", cfg.StreamName)
	}
	if cfg.ExtractorTimeout != 5*time.Second || cfg.SummarizerTimeout != 7*time.Second {
		t.Errorf("timeouts = %v/%v, quería 5s/7s", cfg.ExtractorTimeout, cfg.SummarizerTimeout)
	}
	if cfg.ExtractMaxInFlight != 3 || cfg.SummaryMaxInFlight != 1 {
		t.Errorf("in-flight = %d/%d, quería 3/1", cfg.ExtractMaxInFlight, cfg.SummaryMaxInFlight)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, quería debug", cfg.LogLevel)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]string{
		"HTTP_PORT":             "0",
		"MAX_DOCUMENT_BYTES":    "-1",
		"EXTRACTOR_TIMEOUT":     "30",
		"SUMMARIZER_TIMEOUT":    "1h30m99x",
		"EXTRACT_MAX_IN_FLIGHT": "0",
		"SUMMARY_MAX_IN_FLIGHT": "-2",
	}

	for name, value := range cases {
		t.Run(name+"="+value, func(t *testing.T) {
			_, err := load(envMap(validEnv(map[string]string{name: value})))
			if err == nil {
				t.Fatalf("load() con %s=%s no falló, quería error", name, value)
			}
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("error = %q, quería que mencione %s", err, name)
			}
		})
	}
}
