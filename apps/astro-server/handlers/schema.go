package handlers

import (
	"net/http"

	spec "github.com/astropods/astro-spec"
	"github.com/gin-gonic/gin"
)

// AstroAISpecSchema returns the JSON Schema for the Astro Spec.
func AstroAISpecSchema() gin.HandlerFunc {
	schema := spec.Schema()
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/schema+json", schema)
	}
}
