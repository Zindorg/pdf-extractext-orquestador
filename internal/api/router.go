package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/api/middleware"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// ExtractUseCase es el caso de uso de admisión.
type ExtractUseCase interface {
	Execute(ctx context.Context, in domain.IncomingDocument) (domain.IngestResult, error)
}

// SummaryUseCase es el caso de uso del resumen a demanda.
type SummaryUseCase interface {
	Execute(ctx context.Context, documentID domain.DocumentID) (domain.SummaryRequestResult, error)
}

// ProxyUseCase es el caso de uso que reenvía al Persistidor.
type ProxyUseCase interface {
	Forward(ctx context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error)
}

// Deps son los colaboradores del router. `Ready` reporta si el servicio puede recibir tráfico: con
// Redis caído no se puede publicar y admitir documentos solo produciría errores. `Logger`,
// `RateLimit` y los semáforos acotan el abuso de un servicio sin autenticación (ADR-0007); si se
// omiten, se usan defaults razonables.
type Deps struct {
	Extract       ExtractUseCase
	Summary       SummaryUseCase
	Proxy         ProxyUseCase
	PublicBaseURL string
	Ready         func(ctx context.Context) error
	Logger        *slog.Logger
	RateLimit     *middleware.RateLimitConfig
	ExtractGate   *middleware.Semaphore
	SummaryGate   *middleware.Semaphore
	// MaxDocumentBytes es el tope del archivo admitido (MAX_DOCUMENT_BYTES); 0 usa el default.
	MaxDocumentBytes int
	// TrustedProxies son los proxies (Traefik) cuyo X-Forwarded-For se acepta para conocer la IP del
	// cliente. Vacío: no se confía en ninguno y la IP es la de la conexión.
	TrustedProxies []string
}

// NewRouter arma el router de gin con todas las rutas de openapi.yaml. El prefijo `/api/v1` se
// conserva y no se reescribe (ADR-0005). El modo de gin (release) lo fija main: es global al proceso.
func NewRouter(deps Deps) (*gin.Engine, error) {
	router := gin.New()
	if err := router.SetTrustedProxies(deps.TrustedProxies); err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	if deps.MaxDocumentBytes <= 0 {
		deps.MaxDocumentBytes = application.DefaultMaxDocumentBytes
	}
	// El formulario de admisión, acotado por MaxBytesReader, entra entero en memoria: nada a disco.
	router.MaxMultipartMemory = int64(deps.MaxDocumentBytes) + multipartOverhead
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	if deps.ExtractGate == nil {
		deps.ExtractGate = middleware.NewSemaphore(16)
	}
	if deps.SummaryGate == nil {
		deps.SummaryGate = middleware.NewSemaphore(4)
	}

	// AccessLog va por fuera de Recovery: así una petición que entra en panic también queda en el log
	// de acceso, con su 500.
	router.Use(middleware.Correlation(), middleware.AccessLog(deps.Logger), middleware.Recovery())
	if deps.RateLimit != nil {
		router.Use(middleware.RateLimit(*deps.RateLimit))
	}

	registerRoutes(router, deps)
	return router, nil
}

func registerRoutes(router *gin.Engine, deps Deps) {
	router.GET("/health", handleHealth)
	router.GET("/ready", handleReady(deps.Ready))
	router.GET("/api/v1/health", handleHealth)

	withID := router.Group("/api/v1/documents/:document_id", validateDocumentID)
	withID.GET("", handleProxy(deps.Proxy))
	withID.DELETE("", handleProxy(deps.Proxy))
	withID.GET("/download/original", handleProxy(deps.Proxy))
	withID.GET("/download/summary", handleProxy(deps.Proxy))
	withID.POST("/restore", handleProxy(deps.Proxy))
	withID.POST("/summary", middleware.Concurrency(deps.SummaryGate), handleSummary(deps.Summary, deps.PublicBaseURL))

	router.GET("/api/v1/documents", handleProxy(deps.Proxy))
	router.POST("/api/v1/documents/extract", middleware.Concurrency(deps.ExtractGate), handleExtract(deps.Extract, deps.PublicBaseURL, deps.MaxDocumentBytes))
}

// validateDocumentID es la puerta de borde: un `document_id` que no es UUID no llega ni al caso de
// uso ni al Persistidor.
func validateDocumentID(c *gin.Context) {
	if _, err := uuid.Parse(c.Param("document_id")); err != nil {
		writeError(c, domain.ErrInvalidRequest)
		c.Abort()
		return
	}
	c.Next()
}
