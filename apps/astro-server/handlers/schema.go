package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	spec "github.com/postman/astro/packages/astro-spec"
)

// AstroSpecSchema returns the JSON Schema for astro.yml.
func AstroSpecSchema() gin.HandlerFunc {
	schema := spec.Schema()
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/schema+json", schema)
	}
}
