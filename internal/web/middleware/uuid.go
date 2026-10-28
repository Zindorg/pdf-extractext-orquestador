package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ValidateUUID(param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param(param)
		if _, err := uuid.Parse(id); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Next()
	}
}
