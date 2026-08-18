package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// deploymentAccess holds the authenticated caller and the deployment addressed
// by the :id path parameter.
type deploymentAccess struct {
	Deployment   *deploymentstore.Deployment
	DeploymentID string
	UserID       string
}

// resolveDeploymentAccess authenticates the caller, loads the deployment named
// by :id, and confirms account membership. It writes the matching error response
// and returns false when the caller should stop. Richer per-area resolvers such
// as resolveLangfuseContext and resolveDeploymentContext build on it.
func resolveDeploymentAccess(
	c *gin.Context,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
) (*deploymentAccess, bool) {
	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return nil, false
	}

	dep, err := deploymentStore.GetDeploymentByID(c.Param("id"))
	if err != nil || dep == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return nil, false
	}

	isMember, err := accountStore.IsMember(dep.AccountID, user.ID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return nil, false
	}

	return &deploymentAccess{
		Deployment:   dep,
		DeploymentID: dep.ID,
		UserID:       user.ID,
	}, true
}
