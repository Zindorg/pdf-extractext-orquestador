package router

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/handlers"
	webmw "github.com/Zindorg/pdf-extractext-orquestador/internal/web/middleware"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/render"
)

// Deps son las dependencias de la interfaz web.
type Deps struct {
	Extract       *application.ExtractUseCase
	Summary       *application.SummaryUseCase
	Proxy         *application.ProxyUseCase
	PublicBaseURL string
	Logger        *slog.Logger
}

// Register agrega las rutas de la interfaz web al router.
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

	// Las cabeceras de seguridad van antes de registrar las rutas web, para que cubran también los
	// estáticos y la página 404. Las rutas de la API, registradas antes, no se tocan.
	r.Use(webmw.SecurityHeaders())

	static := r.Group("/static", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Next()
	})
	static.StaticFS("/", render.StaticFS())

	r.GET("/healthz", handlers.Healthz)
	r.GET("/", h.Home)
	r.GET("/documents", h.List)
	r.GET("/documents/:id", webmw.ValidateUUID("id"), h.Detail)
	r.GET("/documents/:id/text", webmw.ValidateUUID("id"), h.FullText)
	r.GET("/documents/:id/download/original", webmw.ValidateUUID("id"), h.DownloadOriginal)
	r.GET("/documents/:id/download/summary", webmw.ValidateUUID("id"), h.DownloadSummary)

	r.POST("/upload", webmw.CSRF(), h.Upload)
	r.POST("/documents/:id/summary", webmw.CSRF(), webmw.ValidateUUID("id"), h.RequestSummary)
	r.POST("/documents/:id/delete", webmw.CSRF(), webmw.ValidateUUID("id"), h.Delete)
	r.POST("/documents/:id/restore", webmw.CSRF(), webmw.ValidateUUID("id"), h.Restore)

	r.NoRoute(h.NotFound)
	return nil
}
