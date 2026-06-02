package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

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
//  1. Principal resolution — user → {user_id, account_ids}; slack → look up
//     the linked WorkOS user via slack_identity_mappings (when scope is
//     supplied), else fall back to the deployment's owning account.
//     Resolution runs before the anyone-grant short-circuit so the response
//     can always carry the resolved identity downstream for trace
//     attribution, not just on grant-matched paths.
//  2. Anyone short-circuit — when an `anyone` grant exists, allow without
//     running MatchesGrant; the resolved identity from step 1 still flows
//     through to the response.
//  3. MatchesGrant against the resolved candidates, plus the transitional
//     no-grants → owner-account fallback (per-adapter).
//
// Returns 200 {allowed, user_id, slack_user_id, slack_team_id} on every
// authoritative answer.
//   - `user_id`       — canonical WorkOS user_id when one is known. For
//     identity_type=user it's the input echoed back; for identity_type=slack
//     it's the linked WorkOS user (empty when no mapping exists). Only
//     populated when allowed=true (denials don't leak mapping state).
//   - `slack_user_id` / `slack_team_id` — echoed for identity_type=slack so
//     the messaging container can attribute unlinked Slack users to a
//     namespaced trace userId instead of dropping them onto Unattributed.
//     Only populated when allowed=true.
//
// Returns 4xx for malformed inputs and 5xx for server-side failures.
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
		// the adapter. Otherwise reject malformed inputs (one of
		// identity_type/identity_id supplied without the other).
		if (identityType == "") != (identityID == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "identity_type and identity_id must be supplied together"})
			return
		}

		// Step 1: principal resolution. Runs before the anyone-grant check
		// because the resolved identity flows into the response regardless of
		// the path that ultimately decides `allowed`. Anonymous callers (no
		// identity supplied) short-circuit inside resolveCandidates with
		// zero queries.
		//
		// Trade-off: this is intentional even though it costs the common
		// `slack: anyone-grant` path an extra indexed lookup on
		// slack_identity_mappings (and, on hit, an account_members lookup) —
		// 3-4 queries vs the pre-change 1. The cost is small (sub-ms,
		// covered by idx_slack_identity_mappings_lookup) and bought once per
		// 60s cache window in the messaging container; the alternative
		// (resolve after anyone short-circuit) would force a second
		// request to attribute the trace, doubling round-trips on the very
		// path that benefits most from attribution. If this becomes hot
		// enough to matter, the lookup can move behind a fast path that
		// skips resolution when identityType=="" (already handled inside
		// resolveCandidates as a zero-query short-circuit).
		candidates, resolvedUserID, err := resolveCandidates(authStore, slackStore, deploymentID, identityType, identityID, identityScope)
		if err != nil {
			log.Error("authorize: failed to resolve identity",
				"deployment_id", deploymentID,
				"identity_type", identityType,
				"error", err,
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
			return
		}

		// Step 2: anyone short-circuit. Skips MatchesGrant + fallback when an
		// open grant exists, but the resolved identity from step 1 still
		// rides along in the response.
		anyone, err := authStore.HasAnyoneGrant(deploymentID, adapter)
		if err != nil {
			log.Error("authorize: anyone-grant lookup failed",
				"deployment_id", deploymentID, "adapter", adapter, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
			return
		}

		allowed := anyone
		if !allowed {
			// Step 3: grant lookup against resolved candidates.
			allowed, err = authStore.MatchesGrant(deploymentID, candidates, adapter)
			if err != nil {
				log.Error("authorize: grant lookup failed",
					"deployment_id", deploymentID,
					"adapter", adapter,
					"error", err,
				)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
				return
			}
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

		// Live-ingest into the Slack user directory so Insights can join
		// historical bare-form Langfuse userIds to a team_id for the
		// `slack://` deep link. Only fires when allowed=true — a denied
		// principal isn't a member of this deployment's universe and
		// shouldn't pollute the directory used for click-through.
		//
		// Fire-and-forget: errors don't fail authz, and the in-process
		// dedupe (UpsertObserved) means a chatty workspace only touches
		// Postgres once per (team, user) per pod. The goroutine is
		// intentionally detached from c.Request.Context() — using the
		// request context would cancel the write when a slow client
		// disconnects, leaving the directory stale. A fresh background
		// context with a tight 5s timeout caps the goroutine's lifetime
		// so a hung DB doesn't leak forever. Safe to outlive the
		// request: the UPSERT is idempotent.
		if allowed && slackStore != nil && identityType == authorizationstore.IdentityTypeSlack && identityScope != "" && identityID != "" {
			go func(teamID, slackUserID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := slackStore.UpsertObserved(ctx, teamID, slackUserID); err != nil {
					log.Warn("authorize: upsert observed slack identity failed",
						"team_id", teamID, "error", err)
				}
			}(identityScope, identityID)
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

		resp := gin.H{"allowed": allowed}
		// Identity fields are only surfaced on allowed responses to avoid
		// leaking mapping state for principals the deployment denies.
		if allowed {
			if resolvedUserID != "" {
				resp["user_id"] = resolvedUserID
			}
			if identityType == authorizationstore.IdentityTypeSlack {
				resp["slack_user_id"] = identityID
				resp["slack_team_id"] = identityScope
			}
		}
		c.JSON(http.StatusOK, resp)
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
		if c.Type == authorizationstore.SubjectTypeOrg && c.ID == ownerAccountID {
			return true, nil
		}
	}
	return false, nil
}

// resolveCandidates turns an (identity_type, identity_id, identity_scope)
// triple into the set of subjects the grant lookup should match against,
// plus the canonical WorkOS user_id when one is identifiable.
//
//   - user  → the user_id itself plus every account the user is a member of;
//     resolved user_id echoes identityID.
//   - slack → the deployment's owning account, plus (when scope=team_id is
//     provided and the user has linked their slack identity) the linked
//     WorkOS user_id and that user's accounts. The owning-account
//     candidate is always emitted so `org` and `anyone` grants keep
//     matching for unmapped slack users. Resolved user_id is the linked
//     WorkOS user, or empty when no mapping exists.
//   - ""    → empty set, empty user_id; only the anyone short-circuit can succeed.
//   - other → empty set with an error to make the caller surface a 4xx.
func resolveCandidates(authStore *authorizationstore.Store, slackStore *slackidentity.Store, deploymentID, identityType, identityID, identityScope string) ([]authorizationstore.Subject, string, error) {
	switch identityType {
	case "":
		return nil, "", nil

	case authorizationstore.IdentityTypeUser:
		accountIDs, err := authStore.AccountIDsForUser(identityID)
		if err != nil {
			return nil, "", err
		}
		out := make([]authorizationstore.Subject, 0, 1+len(accountIDs))
		out = append(out, authorizationstore.Subject{Type: authorizationstore.SubjectTypeUser, ID: identityID})
		for _, aid := range accountIDs {
			out = append(out, authorizationstore.Subject{Type: authorizationstore.SubjectTypeOrg, ID: aid})
		}
		return out, identityID, nil

	case authorizationstore.IdentityTypeSlack:
		accountID, err := authStore.DeploymentAccountID(deploymentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Token is signed but deployment was deleted.
				return nil, "", nil
			}
			return nil, "", err
		}
		out := []authorizationstore.Subject{
			{Type: authorizationstore.SubjectTypeOrg, ID: accountID},
		}
		var resolvedUserID string
		// When the messaging container forwards a team_id, try to resolve
		// the slack user to a linked WorkOS user. Mapping miss is benign —
		// `org` and `anyone` grants still match via the owning-account
		// candidate above. Mapping store errors propagate (5xx) rather
		// than being silently treated as "not linked".
		if slackStore != nil && identityScope != "" && identityID != "" {
			res, err := slackStore.Lookup(identityScope, identityID)
			if err != nil {
				return nil, "", err
			}
			if res.Found {
				resolvedUserID = res.WorkOSUserID
				out = append(out, authorizationstore.Subject{
					Type: authorizationstore.SubjectTypeUser,
					ID:   res.WorkOSUserID,
				})
				accountIDs, err := authStore.AccountIDsForUser(res.WorkOSUserID)
				if err != nil {
					return nil, "", err
				}
				for _, aid := range accountIDs {
					if aid == accountID {
						// Already in the candidate set as the owning
						// account; skip the duplicate.
						continue
					}
					out = append(out, authorizationstore.Subject{
						Type: authorizationstore.SubjectTypeOrg,
						ID:   aid,
					})
				}
			}
		}
		return out, resolvedUserID, nil

	default:
		return nil, "", errors.New("unknown identity_type")
	}
}
