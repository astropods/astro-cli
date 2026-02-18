package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	spec "github.com/postman/astro/packages/astro-spec"
)

// AstroAISpecSchema returns the JSON Schema for astroai.yml.
func AstroAISpecSchema() gin.HandlerFunc {
	schema := spec.Schema()
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/schema+json", schema)
	}
}
