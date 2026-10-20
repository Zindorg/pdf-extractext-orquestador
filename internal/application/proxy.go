package application

import (
	"context"
	"strings"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// listPath es la única colección que el proxy reenvía. El resto de las rutas documentadas cuelga de
// un documento concreto, así que parten de este prefijo.
const listPath = "/api/v1/documents"

// ProxyUseCase reenvía al Persistidor solo las rutas que el Orquestador publica. La allowlist vive
// acá y no en el adaptador: es una decisión de qué expone el Orquestador, no de cómo se habla con el
// Persistidor (openapi.yaml, delta #10).
type ProxyUseCase struct {
	persister domain.PersisterProxy
}

func NewProxyUseCase(persister domain.PersisterProxy) *ProxyUseCase {
	return &ProxyUseCase{persister: persister}
}

// Forward valida el método y la ruta contra la allowlist y, si pasa, reenvía la petición tal como
// llegó: path y query intactos, sin reescribir el prefijo `/api/v1` (ADR-0005). Un rechazo no toca
// al Persistidor: no se gasta una llamada para que el otro lado conteste lo que ya sabemos.
func (uc *ProxyUseCase) Forward(ctx context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error) {
	if !allowed(req.Method, req.Path) {
		return domain.PersisterResponse{}, domain.ErrInvalidRequest
	}
	return uc.persister.Do(ctx, req)
}

// allowed reconoce las rutas documentadas. No se admite un prefijo a ciegas: `GET
// /api/v1/documents/algo/inventado` no está en el contrato y reenviarlo expondría del Persistidor
// rutas que el Orquestador no publica.
func allowed(method, path string) bool {
	if path == listPath {
		return method == "GET"
	}
	if !strings.HasPrefix(path, listPath+"/") {
		return false
	}

	rest := strings.TrimPrefix(path, listPath+"/")
	if rest == "" {
		return false
	}

	if idx := strings.Index(rest, "/"); idx >= 0 {
		return serviceRoute(method, rest[idx+1:])
	}

	switch method {
	case "GET", "DELETE":
		return true
	default:
		return false
	}
}

// serviceRoute cubre los sufijos que cuelgan de un documento: descargas y restauración.
func serviceRoute(method, rest string) bool {
	switch rest {
	case "download/original", "download/summary":
		return method == "GET"
	case "restore":
		return method == "POST"
	default:
		return false
	}
}
