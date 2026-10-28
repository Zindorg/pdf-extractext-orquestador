package handlers

import (
	"log/slog"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/render"
)

type Deps struct {
	Extract       *application.ExtractUseCase
	Summary       *application.SummaryUseCase
	Proxy         *application.ProxyUseCase
	PublicBaseURL string
	Logger        *slog.Logger
}

type Handlers struct {
	deps     Deps
	renderer *render.Renderer
}

func New(d Deps, r *render.Renderer) *Handlers {
	return &Handlers{deps: d, renderer: r}
}
