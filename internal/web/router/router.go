package router

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/handlers"
	webmw "github.com/Zindorg/pdf-extractext-orquestador/internal/web/middleware"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/render"
)

type Deps struct {
	Extract       *application.ExtractUseCase
	Summary       *application.SummaryUseCase
	Proxy         *application.ProxyUseCase
	PublicBaseURL string
	Logger        *slog.Logger
}

func Register(r *gin.Engine, d Deps) error {
	rend, err := render.New()
	if err != nil {
		return err
	}
	h := handlers.New(handlers.Deps{
		Extract:       d.Extract,
		Summary:       d.Summary,
		Proxy:         d.Proxy,
		PublicBaseURL: d.PublicBaseURL,
		Logger:        d.Logger,
	}, rend)

	// static
	r.StaticFS("/static", render.StaticFS())

	r.Use(webmw.SecurityHeaders())

	r.GET("/healthz", handlers.Healthz)
	r.GET("/", h.Home)
	r.GET("/documents", h.List)
	r.GET("/documents/:id", webmw.ValidateUUID("id"), h.Detail)
	r.GET("/documents/:id/download/original", webmw.ValidateUUID("id"), h.DownloadOriginal)
	r.GET("/documents/:id/download/summary", webmw.ValidateUUID("id"), h.DownloadSummary)

	r.POST("/upload", webmw.CSRF(), h.Upload)
	r.POST("/documents/:id/summary", webmw.CSRF(), webmw.ValidateUUID("id"), h.RequestSummary)
	r.POST("/documents/:id/delete", webmw.CSRF(), webmw.ValidateUUID("id"), h.Delete)
	r.POST("/documents/:id/restore", webmw.CSRF(), webmw.ValidateUUID("id"), h.Restore)

	return nil
}
