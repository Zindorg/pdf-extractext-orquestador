package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRF protege los métodos mutantes sin sesiones ni tokens. Orden de decisión:
//  1. Sec-Fetch-Site: lo fija el navegador y el JavaScript de una página no puede alterarlo.
//     "same-origin" pasa; "cross-site" y "same-site" se rechazan.
//  2. Origin, ignorando el valor literal "null": un formulario HTML enviado con
//     Referrer-Policy: no-referrer manda "Origin: null" aunque sea del mismo origen.
//  3. Referer.
//
// Sin ninguna evidencia de mismo origen, se rechaza.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		switch c.GetHeader("Sec-Fetch-Site") {
		case "same-origin":
			c.Next()
			return
		case "cross-site", "same-site":
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		if origin := c.GetHeader("Origin"); origin != "" && origin != "null" {
			if sameHost(origin, c.Request.Host) {
				c.Next()
				return
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		if referer := c.GetHeader("Referer"); referer != "" {
			if sameHost(referer, c.Request.Host) {
				c.Next()
				return
			}
		}
		c.AbortWithStatus(http.StatusForbidden)
	}
}

// sameHost compara host:puerto de la URL recibida con el Host de la petición.
func sameHost(raw, host string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
