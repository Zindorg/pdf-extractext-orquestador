package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func do(method, host string, h map[string]string) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, "/t", CSRF(), func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(method, "/t", nil)
	req.Host = host
	for k, v := range h {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestCSRF(t *testing.T) {
	const host = "pdf-extractext.universidad.localhost"
	cases := []struct {
		name   string
		method string
		hdr    map[string]string
		want   int
	}{
		{"GET libre", "GET", nil, 200},
		{"POST sin evidencia", "POST", nil, 403},
		{"Origin ajeno", "POST", map[string]string{"Origin": "https://evil.com"}, 403},
		{"Origin propio", "POST", map[string]string{"Origin": "https://" + host}, 200},
		{"Referer propio", "POST", map[string]string{"Referer": "https://" + host + "/"}, 200},
		{"Referer ajeno", "POST", map[string]string{"Referer": "https://evil.com/"}, 403},
		// El caso del bug: formulario HTML con Referrer-Policy: no-referrer.
		{"Origin null + Sec-Fetch-Site same-origin", "POST", map[string]string{"Origin": "null", "Sec-Fetch-Site": "same-origin"}, 200},
		{"Origin null sin Sec-Fetch-Site", "POST", map[string]string{"Origin": "null"}, 403},
		{"Sec-Fetch-Site cross-site", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://" + host}, 403},
		{"Sec-Fetch-Site same-site", "POST", map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"DELETE ajeno", "DELETE", map[string]string{"Origin": "https://evil.com"}, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := do(tc.method, host, tc.hdr); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
