package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
// Redis caído no se puede publicar y admitir documentos solo produciría errores.
type Deps struct {
	Extract       ExtractUseCase
	Summary       SummaryUseCase
	Proxy         ProxyUseCase
	PublicBaseURL string
	Ready         func(ctx context.Context) error
}

// NewRouter arma el router de gin con todas las rutas de openapi.yaml. El prefijo `/api/v1` se
// conserva y no se reescribe (ADR-0005).
func NewRouter(deps Deps) *gin.Engine {
	router := gin.New()
	registerRoutes(router, deps)
	return router
}

func registerRoutes(router *gin.Engine, deps Deps) {
	router.GET("/health", handleHealth)
	router.GET("/ready", handleReady(deps.Ready))

	withID := router.Group("/api/v1/documents/:document_id", validateDocumentID)
	withID.GET("", handleProxy(deps.Proxy))
	withID.DELETE("", handleProxy(deps.Proxy))
	withID.GET("/download/original", handleProxy(deps.Proxy))
	withID.GET("/download/summary", handleProxy(deps.Proxy))
	withID.POST("/restore", handleProxy(deps.Proxy))
	withID.POST("/summary", handleSummary(deps.Summary, deps.PublicBaseURL))

	router.GET("/api/v1/documents", handleProxy(deps.Proxy))
	router.POST("/api/v1/documents/extract", handleExtract(deps.Extract, deps.PublicBaseURL))
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
