// Package config carga y valida la configuración del servicio desde el entorno (12-Factor).
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config es la configuración ya validada del servicio. Se construye una sola vez, al arrancar.
type Config struct {
	HTTPPort         int
	PublicBaseURL    string
	MaxDocumentBytes int

	ExtractorURL      string
	SummarizerURL     string
	PersisterURL      string
	ExtractorTimeout  time.Duration
	SummarizerTimeout time.Duration
	PersisterTimeout  time.Duration
	// PersisterMaxResponseBytes acota lo que se lee de una respuesta del Persistidor (descargas).
	PersisterMaxResponseBytes int64

	// RedisURL es una URL redis:// (admite contraseña y base). Si solo se definió REDIS_ADDR, se
	// arma a partir de ella.
	RedisURL     string
	StreamName   string
	StreamMaxLen int64

	ExtractMaxInFlight int
	SummaryMaxInFlight int
	// RateLimitPerMinute es el límite por IP de cliente; 0 lo desactiva.
	RateLimitPerMinute int
	// TrustedProxies son las redes (CIDR) o IPs de los proxies cuyo X-Forwarded-For se acepta.
	TrustedProxies []string

	LogLevel string

	// Registro en Consul. ConsulAddr vacía desactiva el registro (desarrollo y tests).
	ConsulAddr     string
	ConsulToken    string
	ServiceName    string
	ServiceAddress string
	Version        string
}

// Load lee la configuración del entorno del proceso y la valida. Falla rápido: si falta o es inválida
// una variable, el servicio no arranca en lugar de arrancar roto.
func Load() (Config, error) {
	return load(os.Getenv)
}

// load es la implementación testeable: recibe el lector de variables para no depender del proceso.
// Junta todos los problemas en un solo error, en un orden fijo, para corregirlos de una vez.
func load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}
	cfg := Config{
		PublicBaseURL: r.required("PUBLIC_BASE_URL"),
		ExtractorURL:  r.required("EXTRACTOR_URL"),
		SummarizerURL: r.required("SUMMARIZER_URL"),
		PersisterURL:  r.required("PERSISTER_URL"),

		HTTPPort:         r.intRange("HTTP_PORT", 8080, 1, 65535),
		MaxDocumentBytes: r.intRange("MAX_DOCUMENT_BYTES", 10<<20, 1, 1<<30),

		// El Resumidor es holgado por diseño: el modelo local serializa lento (workers.md).
		ExtractorTimeout:          r.duration("EXTRACTOR_TIMEOUT", 30*time.Second),
		SummarizerTimeout:         r.duration("SUMMARIZER_TIMEOUT", 120*time.Second),
		PersisterTimeout:          r.duration("PERSISTER_TIMEOUT", 15*time.Second),
		PersisterMaxResponseBytes: int64(r.intRange("PERSISTER_MAX_RESPONSE_BYTES", 32<<20, 1, 1<<30)),

		StreamName:   r.str("STREAM_NAME", "document-events"),
		StreamMaxLen: int64(r.intRange("STREAM_MAX_LEN", 10000, 0, 1<<31-1)),

		ExtractMaxInFlight: r.intRange("EXTRACT_MAX_IN_FLIGHT", 16, 1, 10000),
		SummaryMaxInFlight: r.intRange("SUMMARY_MAX_IN_FLIGHT", 4, 1, 10000),
		RateLimitPerMinute: r.intRange("RATE_LIMIT_PER_MINUTE", 120, 0, 1_000_000),
		TrustedProxies:     r.networks("TRUSTED_PROXIES"),

		LogLevel: strings.ToLower(r.str("LOG_LEVEL", "info")),

		ConsulAddr:     strings.TrimSpace(getenv("CONSUL_HTTP_ADDR")),
		ConsulToken:    strings.TrimSpace(getenv("CONSUL_HTTP_TOKEN")),
		ServiceName:    r.str("CONSUL_SERVICE_NAME", "pdf-extractext-orquestador"),
		ServiceAddress: strings.TrimSpace(getenv("SERVICE_ADDRESS")),
		Version:        r.str("VERSION", "dev"),
	}
	cfg.RedisURL = r.redisURL()

	if cfg.PublicBaseURL != "" {
		r.check(validBaseURL(cfg.PublicBaseURL), "PUBLIC_BASE_URL inválida: %q, debe ser una URL absoluta http(s)", cfg.PublicBaseURL)
	}
	for name, raw := range map[string]string{"EXTRACTOR_URL": cfg.ExtractorURL, "SUMMARIZER_URL": cfg.SummarizerURL, "PERSISTER_URL": cfg.PersisterURL} {
		if raw != "" {
			r.check(validBaseURL(raw), "%s inválida: %q, debe ser una URL absoluta http(s)", name, raw)
		}
	}

	if err := errors.Join(r.errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// reader acumula los errores de lectura para reportarlos todos juntos.
type reader struct {
	getenv func(string) string
	errs   []error
}

func (r *reader) check(ok bool, format string, args ...any) {
	if !ok {
		r.errs = append(r.errs, fmt.Errorf(format, args...))
	}
}

func (r *reader) required(name string) string {
	value := strings.TrimSpace(r.getenv(name))
	r.check(value != "", "falta la variable obligatoria %s", name)
	return value
}

func (r *reader) str(name, fallback string) string {
	if value := strings.TrimSpace(r.getenv(name)); value != "" {
		return value
	}
	return fallback
}

func (r *reader) intRange(name string, fallback, minimum, maximum int) int {
	raw := strings.TrimSpace(r.getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		r.check(false, "%s inválida: %q, debe ser un entero", name, raw)
		return fallback
	}
	r.check(value >= minimum && value <= maximum, "%s inválida: %d, debe estar entre %d y %d", name, value, minimum, maximum)
	return value
}

func (r *reader) duration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(r.getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		r.check(false, "%s inválida: %q, debe ser una duración con unidad (ej. 30s)", name, raw)
		return fallback
	}
	r.check(value > 0, "%s inválida: %q, debe ser mayor que cero", name, raw)
	return value
}

// networks lee una lista separada por comas de CIDRs o IPs.
func (r *reader) networks(name string) []string {
	raw := strings.TrimSpace(r.getenv(name))
	if raw == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		_, _, cidrErr := net.ParseCIDR(item)
		r.check(cidrErr == nil || net.ParseIP(item) != nil, "%s inválida: %q no es un CIDR ni una IP", name, item)
		out = append(out, item)
	}
	return out
}

// redisURL toma REDIS_URL; si no está, arma la URL desde REDIS_ADDR (compatibilidad). Una de las dos
// es obligatoria.
func (r *reader) redisURL() string {
	if raw := strings.TrimSpace(r.getenv("REDIS_URL")); raw != "" {
		u, err := url.Parse(raw)
		r.check(err == nil && (u.Scheme == "redis" || u.Scheme == "rediss") && u.Host != "",
			"REDIS_URL inválida: debe tener la forma redis://[:password@]host:port[/db]")
		return raw
	}
	if addr := strings.TrimSpace(r.getenv("REDIS_ADDR")); addr != "" {
		return "redis://" + addr
	}
	r.check(false, "falta la variable obligatoria REDIS_URL (o REDIS_ADDR)")
	return ""
}

func validBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
