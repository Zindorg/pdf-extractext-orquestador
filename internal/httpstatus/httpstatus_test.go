package httpstatus

import (
	"net/http"
	"testing"
)

func TestForCode(t *testing.T) {
	cases := map[string]int{
		"invalid_request":       http.StatusBadRequest,
		"document_not_found":    http.StatusNotFound,
		"summary_pending":       http.StatusConflict,
		"payload_too_large":     http.StatusRequestEntityTooLarge,
		"extraction_failed":     http.StatusUnprocessableEntity,
		"rate_limited":          http.StatusTooManyRequests,
		"summary_failed":        http.StatusBadGateway,
		"extractor_unavailable": http.StatusServiceUnavailable,
		"persister_unavailable": http.StatusServiceUnavailable,
	}
	for code, want := range cases {
		if got, ok := ForCode(code); !ok || got != want {
			t.Errorf("ForCode(%q) = %d, %v; quería %d", code, got, ok, want)
		}
	}
	if _, ok := ForCode("inventado"); ok {
		t.Error("un código fuera del vocabulario no debe tener status")
	}
}
