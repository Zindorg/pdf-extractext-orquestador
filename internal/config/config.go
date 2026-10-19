// Package config carga y valida la configuración del servicio desde el entorno (12-Factor).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config es la configuración ya validada del servicio. Se construye una sola vez, al arrancar.
type Config struct {
	HTTPPort           int
	PublicBaseURL      string
	MaxDocumentBytes   int
	ExtractorURL       string
	SummarizerURL      string
	PersisterURL       string
	RedisAddr          string
	StreamName         string
	ExtractorTimeout   time.Duration
	SummarizerTimeout  time.Duration
	ExtractMaxInFlight int
	SummaryMaxInFlight int
	LogLevel           string
}

// Load lee la configuración del entorno del proceso y la valida. Falla rápido: si falta o es inválida
// una variable, el servicio no arranca en lugar de arrancar roto.
func Load() (Config, error) {
	return load(os.Getenv)
}

// load es la implementación testeable: recibe el lector de variables para no depender del proceso.
func load(getenv func(string) string) (Config, error) {
	var cfg Config

	required := map[string]*string{
		"PUBLIC_BASE_URL": &cfg.PublicBaseURL,
		"EXTRACTOR_URL":   &cfg.ExtractorURL,
		"SUMMARIZER_URL":  &cfg.SummarizerURL,
		"PERSISTER_URL":   &cfg.PersisterURL,
		"REDIS_ADDR":      &cfg.RedisAddr,
	}
	for name, dst := range required {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			return Config{}, fmt.Errorf("falta la variable obligatoria %s", name)
		}
		*dst = value
	}

	var err error
	if cfg.HTTPPort, err = intVar(getenv, "HTTP_PORT", 8080); err != nil {
		return Config{}, err
	}
	if cfg.HTTPPort < 1 || cfg.HTTPPort > 65535 {
		return Config{}, fmt.Errorf("HTTP_PORT inválida: %d, debe estar entre 1 y 65535", cfg.HTTPPort)
	}

	if cfg.MaxDocumentBytes, err = intVar(getenv, "MAX_DOCUMENT_BYTES", 10<<20); err != nil {
		return Config{}, err
	}
	if cfg.MaxDocumentBytes <= 0 {
		return Config{}, fmt.Errorf("MAX_DOCUMENT_BYTES inválida: %d, debe ser mayor que cero", cfg.MaxDocumentBytes)
	}

	if cfg.ExtractMaxInFlight, err = intVar(getenv, "EXTRACT_MAX_IN_FLIGHT", 16); err != nil {
		return Config{}, err
	}
	if cfg.ExtractMaxInFlight <= 0 {
		return Config{}, fmt.Errorf("EXTRACT_MAX_IN_FLIGHT inválida: %d, debe ser mayor que cero", cfg.ExtractMaxInFlight)
	}

	if cfg.SummaryMaxInFlight, err = intVar(getenv, "SUMMARY_MAX_IN_FLIGHT", 4); err != nil {
		return Config{}, err
	}
	if cfg.SummaryMaxInFlight <= 0 {
		return Config{}, fmt.Errorf("SUMMARY_MAX_IN_FLIGHT inválida: %d, debe ser mayor que cero", cfg.SummaryMaxInFlight)
	}

	// El Resumidor es holgado por diseño: el modelo local serializa lento (workers.md).
	if cfg.ExtractorTimeout, err = durationVar(getenv, "EXTRACTOR_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.SummarizerTimeout, err = durationVar(getenv, "SUMMARIZER_TIMEOUT", 120*time.Second); err != nil {
		return Config{}, err
	}

	cfg.StreamName = strings.TrimSpace(getenv("STREAM_NAME"))
	if cfg.StreamName == "" {
		cfg.StreamName = "document-events"
	}
	cfg.LogLevel = strings.TrimSpace(getenv("LOG_LEVEL"))
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}

func intVar(getenv func(string) string, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s inválida: %q, debe ser un entero", name, raw)
	}
	return value, nil
}

func durationVar(getenv func(string) string, name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s inválida: %q, debe ser una duración con unidad (ej. 30s)", name, raw)
	}
	return value, nil
}
