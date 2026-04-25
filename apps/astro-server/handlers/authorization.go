package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// CheckDeploymentAuthorization is the messaging container's callback. It
// answers "is this principal allowed to use this deployment over this adapter."
//
// Auth: the deploy token (validated by RequireDeployToken middleware) supplies
// the deployment_id. The body of the request supplies the identity to check.
//
// Inputs (query params):
//   - identity_type: "user" | "slack" | empty (anonymous)
//   - identity_id:   the WorkOS user ID, slack user ID, or empty
//   - adapter:       "web" | "slack"
//
// Flow:
//  1. Anyone short-circuit — handled inside store.IsAllowed.
//  2. Principal resolution — user → {user_id, account_ids}; slack → look up
//     the deployment's owning account from the deployments row.
//  3. store.IsAllowed runs the grant lookup and returns the boolean.
//
// Returns 200 {allowed: bool} on every authoritative answer. Returns 4xx for
// malformed inputs and 5xx for server-side failures.
func CheckDeploymentAuthorization(log *logger.Logger, authStore *authorizationstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		deploymentID := middleware.DeploymentIDFromContext(c)
		if deploymentID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing deployment context"})
			return
		}

		identityType := c.Query("identity_type")
		identityID := c.Query("identity_id")
		adapter := c.Query("adapter")

		if adapter != authorizationstore.AdapterWeb && adapter != authorizationstore.AdapterSlack {
			c.JSON(http.StatusBadRequest, gin.H{"error": "adapter must be one of: web, slack"})
			return
		}

		// An empty identity is only valid when an `anyone` grant exists for
		// the adapter; the store's anyone short-circuit handles that.
		// Otherwise reject malformed inputs (one of identity_type/identity_id
		// supplied without the other).
		if (identityType == "") != (identityID == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "identity_type and identity_id must be supplied together"})
			return
		}

		// Step 1 (per spec): anyone short-circuit, before resolving the
		// principal. An anyone grant lets us skip the account_members /
		// deployments lookup entirely — and lets anonymous traffic pass
		// without any identity at all.
		if anyone, err := authStore.HasAnyoneGrant(deploymentID, adapter); err != nil {
			log.Error("authorize: anyone-grant lookup failed",
				"deployment_id", deploymentID, "adapter", adapter, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
			return
		} else if anyone {
			c.JSON(http.StatusOK, gin.H{"allowed": true})
			return
		}

		// Step 2: principal resolution.
		candidates, err := resolveCandidates(authStore, deploymentID, identityType, identityID)
		if err != nil {
			log.Error("authorize: failed to resolve identity",
				"deployment_id", deploymentID,
				"identity_type", identityType,
				"error", err,
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
			return
		}

		// Step 3: grant lookup against resolved candidates.
		allowed, err := authStore.MatchesGrant(deploymentID, candidates, adapter)
		if err != nil {
			log.Error("authorize: grant lookup failed",
				"deployment_id", deploymentID,
				"adapter", adapter,
				"error", err,
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
			return
		}

		// Test case J2: log denials with enough context to debug.
		if !allowed {
			log.Info("authorize: denied",
				"deployment_id", deploymentID,
				"identity_type", identityType,
				"identity_id", identityID,
				"adapter", adapter,
			)
		}

		c.JSON(http.StatusOK, gin.H{"allowed": allowed})
	}
}

// resolveCandidates turns an (identity_type, identity_id) pair into the set of
// subjects the grant lookup should match against.
//
//   - user  → the user_id itself plus every account the user is a member of.
//   - slack → the deployment's owning account (slack identity is opaque, so
//     per-user authorization isn't possible there).
//   - ""    → empty set; only the anyone short-circuit can succeed.
//   - other → empty set with an error to make the caller surface a 4xx.
func resolveCandidates(authStore *authorizationstore.Store, deploymentID, identityType, identityID string) ([]authorizationstore.Subject, error) {
	switch identityType {
	case "":
		return nil, nil

	case authorizationstore.IdentityTypeUser:
		accountIDs, err := authStore.AccountIDsForUser(identityID)
		if err != nil {
			return nil, err
		}
		out := make([]authorizationstore.Subject, 0, 1+len(accountIDs))
		out = append(out, authorizationstore.Subject{Type: authorizationstore.SubjectTypeUser, ID: identityID})
		for _, aid := range accountIDs {
			out = append(out, authorizationstore.Subject{Type: authorizationstore.SubjectTypeAccount, ID: aid})
		}
		return out, nil

	case authorizationstore.IdentityTypeSlack:
		accountID, err := authStore.DeploymentAccountID(deploymentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Token is signed but deployment was deleted.
				return nil, nil
			}
			return nil, err
		}
		return []authorizationstore.Subject{
			{Type: authorizationstore.SubjectTypeAccount, ID: accountID},
		}, nil

	default:
		return nil, errors.New("unknown identity_type")
	}
}
