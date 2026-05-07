package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/gin-gonic/gin"
)

// CheckDeploymentAuthorization is the messaging container's callback. It
// answers "is this principal allowed to use this deployment over this adapter."
//
// Auth: the deploy token (validated by RequireDeployToken middleware) supplies
// the deployment_id. The body of the request supplies the identity to check.
//
// Inputs (query params):
//   - identity_type:  "user" | "slack" | empty (anonymous)
//   - identity_id:    the WorkOS user ID, slack user ID, or empty
//   - identity_scope: adapter-specific disambiguator for identity_id —
//     for slack this is the team_id (slack user_ids are only unique per
//     team). Empty for web.
//   - adapter:        "web" | "slack"
//
// Flow:
//  1. Anyone short-circuit — handled inside store.IsAllowed.
//  2. Principal resolution — user → {user_id, account_ids}; slack → look up
//     the linked WorkOS user via slack_identity_mappings (when scope is
//     supplied), else fall back to the deployment's owning account.
//  3. store.IsAllowed runs the grant lookup and returns the boolean.
//
// Returns 200 {allowed: bool} on every authoritative answer. Returns 4xx for
// malformed inputs and 5xx for server-side failures.
func CheckDeploymentAuthorization(log *logger.Logger, authStore *authorizationstore.Store, slackStore *slackidentity.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		deploymentID := middleware.DeploymentIDFromContext(c)
		if deploymentID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing deployment context"})
			return
		}

		identityType := c.Query("identity_type")
		identityID := c.Query("identity_id")
		identityScope := c.Query("identity_scope")
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
		candidates, err := resolveCandidates(authStore, slackStore, deploymentID, identityType, identityID, identityScope)
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

		// Step 4: transitional fallback for deployments that pre-date this
		// authorization rollout. If the deployment has no grants for *this
		// adapter*, allow members of its owning account — preserving the
		// pre-auth behavior. The fallback is scoped per-adapter so writing a
		// grant on slack doesn't silently lock down web (or vice-versa); it
		// turns off only for the adapter the owner has started configuring.
		if !allowed {
			ownerAllowed, err := tryOwnerFallback(authStore, deploymentID, adapter, candidates)
			if err != nil {
				log.Error("authorize: fallback lookup failed",
					"deployment_id", deploymentID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
				return
			}
			allowed = ownerAllowed
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

// tryOwnerFallback implements the transitional "no grants → owner-account
// access" rule, scoped per-adapter. Returns true only when:
//   - the deployment has zero grant rows for the requested adapter, AND
//   - one of the resolved candidate accounts is the deployment's owner.
//
// Slack candidates always include the owning account (they're resolved from
// the deployments row), so this naturally covers slack pre-existing deploys
// without any extra branching.
func tryOwnerFallback(authStore *authorizationstore.Store, deploymentID, adapter string, candidates []authorizationstore.Subject) (bool, error) {
	hasAny, err := authStore.HasAnyGrants(deploymentID, adapter)
	if err != nil {
		return false, err
	}
	if hasAny {
		return false, nil
	}
	ownerAccountID, err := authStore.DeploymentAccountID(deploymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	for _, c := range candidates {
		if c.Type == authorizationstore.SubjectTypeAccount && c.ID == ownerAccountID {
			return true, nil
		}
	}
	return false, nil
}

// resolveCandidates turns an (identity_type, identity_id, identity_scope)
// triple into the set of subjects the grant lookup should match against.
//
//   - user  → the user_id itself plus every account the user is a member of.
//   - slack → the deployment's owning account, plus (when scope=team_id is
//     provided and the user has linked their slack identity) the linked
//     WorkOS user_id and that user's accounts. The owning-account
//     candidate is always emitted so `org` and `anyone` grants keep
//     matching for unmapped slack users.
//   - ""    → empty set; only the anyone short-circuit can succeed.
//   - other → empty set with an error to make the caller surface a 4xx.
func resolveCandidates(authStore *authorizationstore.Store, slackStore *slackidentity.Store, deploymentID, identityType, identityID, identityScope string) ([]authorizationstore.Subject, error) {
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
		out := []authorizationstore.Subject{
			{Type: authorizationstore.SubjectTypeAccount, ID: accountID},
		}
		// When the messaging container forwards a team_id, try to resolve
		// the slack user to a linked WorkOS user. Mapping miss is benign —
		// `org` and `anyone` grants still match via the owning-account
		// candidate above. Mapping store errors propagate (5xx) rather
		// than being silently treated as "not linked".
		if slackStore != nil && identityScope != "" && identityID != "" {
			res, err := slackStore.Lookup(identityScope, identityID)
			if err != nil {
				return nil, err
			}
			if res.Found {
				out = append(out, authorizationstore.Subject{
					Type: authorizationstore.SubjectTypeUser,
					ID:   res.WorkOSUserID,
				})
				accountIDs, err := authStore.AccountIDsForUser(res.WorkOSUserID)
				if err != nil {
					return nil, err
				}
				for _, aid := range accountIDs {
					if aid == accountID {
						// Already in the candidate set as the owning
						// account; skip the duplicate.
						continue
					}
					out = append(out, authorizationstore.Subject{
						Type: authorizationstore.SubjectTypeAccount,
						ID:   aid,
					})
				}
			}
		}
		return out, nil

	default:
		return nil, errors.New("unknown identity_type")
	}
}
