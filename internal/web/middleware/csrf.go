package middleware

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// CSRF verifica Origin o Referer contra el Host de la petición para métodos mutantes.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin != "" {
			if sameOrigin(origin, c.Request.Host) {
				c.Next()
				return
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		referer := c.GetHeader("Referer")
		if referer != "" {
			if sameOrigin(referer, c.Request.Host) {
				c.Next()
				return
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.AbortWithStatus(http.StatusForbidden)
	}
}

func sameOrigin(header, host string) bool {
	u, err := url.Parse(header)
	if err != nil {
		return false
	}
	if u.Host == host {
		return true
	}
	// comparar solo host sin puerto si vienen distintos formatos
	if h := u.Hostname(); h != "" {
		// host puede tener puerto
		h2 := host
		if colon := indexByte(host, ':'); colon >= 0 {
			// si header trae puerto explícito distinto pero mismo hostname en mismo puerto? Traefik https 443
			// pero comparar hostname contra host base
			if u.Port() == "" {
				// origen sin puerto pero host puede tenerlo? raro
			}
			h2 = host[:colon]
		}
		if h == h2 {
			// si header tiene puerto y coincide con host puerto? o si host es sin puerto
			if u.Port() == "" && h2 == h {
				return true
			}
			if u.Port() != "" {
				// require port match only if header specifies port? or just hostname match in same-origin context
				// be strict by full host when both present
			}
			// try full match again more loosely
			return u.Host == host || (u.Hostname() == h2 && (u.Port() == "" || u.Port() == portOf(host)))
		}
		return false
	}
	return u.Host == host
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func portOf(host string) string {
	if i := indexByte(host, ':'); i >= 0 {
		return host[i+1:]
	}
	return ""
}
