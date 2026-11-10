package redisstreams

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// SetLogger redirige los avisos internos de go-redis (reconexiones, fallos del pool) al logger JSON
// del servicio. Sin esto salen por el log estándar en texto plano, mezclados con el JSON.
func SetLogger(logger *slog.Logger) {
	redis.SetLogger(slogPrinter{logger: logger})
}

// slogPrinter adapta la interfaz Printf de go-redis a slog.
type slogPrinter struct{ logger *slog.Logger }

func (p slogPrinter) Printf(ctx context.Context, format string, v ...any) {
	p.logger.WarnContext(ctx, "redis", "detail", fmt.Sprintf(format, v...))
}
