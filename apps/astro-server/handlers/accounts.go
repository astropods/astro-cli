package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/gin-gonic/gin"
)

// CreateAccountRequest represents the request body for creating an account
type CreateAccountRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required"`
	DisplayName string `json:"display_name"`
	Invitations []struct {
		Value string `json:"value"`
		Kind  string `json:"kind"`
		Role  string `json:"role"`
	} `json:"invitations,omitempty"`
}

func accountDisplayNameMaxLength(accountType string) int {
	if accountType == "organization" {
		return account.OrganizationDisplayNameMaxLength
	}
	return account.DisplayNameMaxLength
}

func accountDisplayNameLengthError(accountType string, maxLength int) string {
	if accountType == "organization" {
		return "organization names cannot exceed " + strconv.Itoa(maxLength) + " characters"
	}
	return "display names cannot exceed " + strconv.Itoa(maxLength) + " characters"
}

// AccountOwner represents the owner's public profile in account responses
type AccountOwner struct {
	FirstName         string `json:"first_name,omitempty"`
	LastName          string `json:"last_name,omitempty"`
	ProfilePictureURL string `json:"profile_picture_url,omitempty"`
}

// AccountResponse represents an account in API responses
type AccountResponse struct {
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Type            string                   `json:"type"`
	DisplayName     string                   `json:"display_name"`
	Owner           *AccountOwner            `json:"owner,omitempty"`
	Invitations     []org.InviteResult       `json:"invitations,omitempty"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
	AccountNumber   *int                     `json:"account_number,omitempty"`
	Bio             string                   `json:"bio,omitempty"`
	Location        string                   `json:"location,omitempty"`
	LocalTimezone   string                   `json:"local_timezone,omitempty"`
	Pronouns        string                   `json:"pronouns,omitempty"`
	Website         string                   `json:"website,omitempty"`
	SocialLinks     []string                 `json:"social_links"`
	BlueprintOrder  []string                 `json:"blueprint_order,omitempty"`
	AvatarURL       string                   `json:"avatar_url,omitempty"`
	AllowedClusters []account.ClusterBinding `json:"allowed_clusters,omitempty"`
}

// AccountOrgResponse represents an org account in the profile orgs list.
type AccountOrgResponse struct {
	Name         string           `json:"name"`
	DisplayName  string           `json:"display_name"`
	AvatarColors *json.RawMessage `json:"avatar_colors,omitempty"`
}

// AccountWithRoleResponse represents an account in the profile response
type AccountWithRoleResponse struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	Type                 string         `json:"type"`
	DisplayName          string         `json:"display_name"`
	WorkOSOrganizationID string         `json:"workos_org_id,omitempty"`
	Agents               []AgentSummary `json:"agents,omitempty"`
}

// AgentSummary represents a brief summary of an agent for the profile response
type AgentSummary struct {
	Name       string `json:"name"`
	Registry   string `json:"registry"`
	Visibility string `json:"visibility"`
	BuildCount int    `json:"build_count"`
}

// ProfileResponse represents the /api/v1/me response
type ProfileResponse struct {
	User     *ProfileUser              `json:"user"`
	Accounts []AccountWithRoleResponse `json:"accounts"`
}

// ProfileUser is a subset of user info for the profile endpoint
type ProfileUser struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
}

// CreateAccount handles POST /api/v1/accounts
// For organization accounts, also creates a WorkOS Organization and links it.
// If billingProvider is non-nil, creates a corresponding billing customer (non-blocking).
func CreateAccount(log *logger.Logger, accountStore *account.AccountStore, orgClient *org.Client, orgSync *org.Sync, memberEmails memberEmailUpserter, billingProvider billing.BillingProvider, auditStore *auditlog.Store, queue notifyQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		// For personal accounts, check user doesn't already have one
		if req.Type == "personal" {
			hasPersonal, err := accountStore.HasPersonalAccount(user.ID)
			if err != nil {
				log.Error("accounts: check personal account failed", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check existing accounts"})
				return
			}
			if hasPersonal {
				c.JSON(http.StatusConflict, gin.H{"error": "you already have a personal account"})
				return
			}
		}

		req.DisplayName = strings.TrimSpace(req.DisplayName)
		displayNameMaxLength := accountDisplayNameMaxLength(req.Type)
		if req.DisplayName != "" && utf8.RuneCountInString(req.DisplayName) > displayNameMaxLength {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request",
				"details": accountDisplayNameLengthError(req.Type, displayNameMaxLength),
			})
			return
		}

		// Organization accounts require a display name
		if req.Type == "organization" && req.DisplayName == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request",
				"details": "display name is required for organization accounts",
			})
			return
		}

		// Validate name format (length, charset, casing, hyphens)
		if err := account.ValidateAccountName(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid account name",
				"details": err.Error(),
			})
			return
		}

		// Check reserved/denied names for user registration
		if err := account.CheckAccountNameRestricted(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid account name",
				"details": err.Error(),
			})
			return
		}

		// Step 1: Create local account (Astro is source of truth)
		acct, err := accountStore.Create(req.Name, req.Type, user.ID, req.DisplayName)
		if err != nil {
			log.Error("accounts: create account failed", "error", err, "name", req.Name)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to create account",
				"details": err.Error(),
			})
			return
		}

		// Best-effort: mirror the creator's WorkOS email locally for dev-tool
		// attribution. Non-fatal — account creation must not fail on this.
		if memberEmails != nil {
			if err := memberEmails.UpsertWorkOS(c.Request.Context(), user.ID, user.Email, user.EmailVerified); err != nil {
				log.Warn("accounts: mirror member email on account create failed", "error", err, "user_id", user.ID)
			}
		}

		// Login skips this sync until a personal account exists; it retries there.
		if req.Type == "personal" && orgSync != nil {
			if err := orgSync.SyncMembershipsForUser(c.Request.Context(), user.ID); err != nil {
				log.Warn("accounts: sync memberships after personal account create failed", "error", err, "user_id", user.ID)
			}
		}

		// Step 2: For org accounts, create WorkOS Organization and link
		if req.Type == "organization" && orgClient != nil {
			ctx := c.Request.Context()

			// Create WorkOS organization with external_id = account.ID
			workosOrg, err := orgClient.CreateOrganization(ctx, req.Name, acct.ID)
			if err != nil {
				log.Error("accounts: create WorkOS organization failed", "error", err, "account_id", acct.ID)
				// Compensating action: delete local account
				_ = accountStore.DeleteByID(acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "failed to create organization",
					"details": err.Error(),
				})
				return
			}

			// Link WorkOS org to local account
			if err := accountStore.SetWorkOSOrganizationID(acct.ID, workosOrg.ID); err != nil {
				log.Error("accounts: link WorkOS org failed", "error", err, "account_id", acct.ID)
				_ = orgClient.DeleteOrganization(ctx, workosOrg.ID)
				_ = accountStore.DeleteByID(acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to link organization"})
				return
			}
			acct.WorkOSOrganizationID = workosOrg.ID

			// Create WorkOS membership for creator as owner
			m, err := orgClient.CreateMembership(ctx, workosOrg.ID, user.ID, "owner")
			if err != nil {
				log.Error("accounts: create WorkOS membership for org creator failed", "error", err, "account_id", acct.ID)
				// Compensating action: clean up WorkOS org and local account
				_ = orgClient.DeleteOrganization(ctx, workosOrg.ID)
				_ = accountStore.DeleteByID(acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "failed to create organization membership",
					"details": err.Error(),
				})
				return
			}
			if err := accountStore.UpsertMemberByWorkosMembershipID(acct.ID, user.ID, m.ID); err != nil {
				log.Warn("accounts: record WorkOS membership for org creator failed, healed at next login",
					"error", err, "account_id", acct.ID, "workos_membership_id", m.ID)
			}
		}

		// Billing customer, rate card, and signup credit are provisioned off the
		// request path. A dropped enqueue is picked up by the hourly sweep.
		// Only a provisioning backend has a worker registered for the job, and
		// the noop provider is non-nil, so assert the seam rather than nil.
		_, provisions := billingProvider.(billing.Provisioner)
		if q, ok := queue.(billingProvisionQueue); ok && provisions {
			if err := q.InsertBillingProvision(c.Request.Context(), acct.ID); err != nil {
				log.Error("accounts: enqueue billing provisioning failed", "error", err, "account_id", acct.ID)
			}
		}

		log.Info("accounts: account created", "id", acct.ID, "name", acct.Name, "type", acct.Type, "user_id", user.ID)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AccountCreate
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Created account " + acct.Name
		evt.Metadata = map[string]any{"type": acct.Type}
		auditStore.LogAsync(log, evt)

		// Welcome the creator. Best-effort — the creator's email was just mirrored
		// above, so AudienceActor resolves; a nil queue (Novu unconfigured) no-ops.
		emitNotify(c, log, queue, notify.AccountWelcome(acct.ID, acct.Name, user.ID))

		resp := AccountResponse{
			ID:          acct.ID,
			Name:        acct.Name,
			Type:        acct.Type,
			DisplayName: acct.DisplayName,

			CreatedAt: acct.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: acct.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		// Send invitations if provided (non-fatal — log failures, still return success)
		if len(req.Invitations) > 0 && acct.WorkOSOrganizationID != "" && orgSync != nil {
			reqs := make([]org.InviteRequest, 0, len(req.Invitations))
			for _, inv := range req.Invitations {
				role := inv.Role
				if role == "" {
					role = "member"
				}
				reqs = append(reqs, org.InviteRequest{
					Kind:     inv.Kind,
					Value:    inv.Value,
					RoleSlug: role,
				})
			}
			results := orgSync.SendBulkInvitations(c.Request.Context(), acct.WorkOSOrganizationID, user.ID, reqs)
			resp.Invitations = results
			for _, r := range results {
				if !r.Success {
					log.Warn("accounts: invitation failed during account creation", "value", r.Value, "error", r.Error)
				}
			}
		}

		c.JSON(http.StatusCreated, resp)
	}
}

// GetAccount handles GET /api/v1/accounts/:account (public)
func GetAccount(log *logger.Logger, accountStore *account.AccountStore, avatarStore *avatar.Store, workos *auth.WorkOSClient, clusters clusterid.Resolver) gin.HandlerFunc {
	bindings := accountStore.Clusters(clusters)
	return func(c *gin.Context) {
		accountName := c.Param("account")

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		socialLinks := acct.SocialLinks
		if socialLinks == nil {
			socialLinks = []string{}
		}

		resp := AccountResponse{
			ID:             acct.ID,
			Name:           acct.Name,
			Type:           acct.Type,
			DisplayName:    acct.DisplayName,
			CreatedAt:      acct.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:      acct.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			AccountNumber:  acct.AccountNumber,
			Bio:            acct.Bio,
			Location:       acct.Location,
			LocalTimezone:  acct.LocalTimezone,
			Pronouns:       acct.Pronouns,
			Website:        acct.Website,
			SocialLinks:    socialLinks,
			BlueprintOrder: acct.BlueprintOrder,
		}

		if avatarStore != nil {
			resp.AvatarURL = avatarStore.AvatarURL(acct.Name, acct.AvatarUpdatedAt)
		}

		if allowed, err := bindings.List(acct.ID); err != nil {
			log.Error("accounts: list account clusters failed", "error", err, "account_id", acct.ID)
		} else {
			resp.AllowedClusters = allowed
		}

		// Best-effort: look up owner profile for personal accounts
		if acct.Type == "personal" {
			if ownerID, err := accountStore.GetFirstMemberUserID(acct.ID); err == nil {
				if user, err := workos.GetUser(c.Request.Context(), ownerID); err == nil {
					resp.Owner = &AccountOwner{
						FirstName:         user.FirstName,
						LastName:          user.LastName,
						ProfilePictureURL: user.ProfilePictureURL,
					}
				}
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// DeleteAccount handles DELETE /api/v1/accounts/:account (owner only)
// Soft-deletes the account, starts best-effort cleanup of account resources,
// and enqueues undeploy jobs for active deployments.
func DeleteAccount(log *logger.Logger, deleter *accountlifecycle.Deleter, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if _, err := deleter.Delete(c.Request.Context(), acct); errors.Is(err, account.ErrAlreadyDeleted) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		} else if err != nil {
			log.Error("accounts: delete account failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete account"})
			return
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AccountDelete
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Deleted account " + acct.Name
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{"message": "account deleted"})
	}
}

// UpdateAccountRequest represents the request body for updating account fields.
// Profile fields use pointer/PATCH semantics: omitting a field leaves the existing value unchanged.
type UpdateAccountRequest struct {
	DisplayName    *string   `json:"display_name"`
	Bio            *string   `json:"bio"`
	Location       *string   `json:"location"`
	LocalTimezone  *string   `json:"local_timezone"`
	Pronouns       *string   `json:"pronouns"`
	Website        *string   `json:"website"`
	SocialLinks    *[]string `json:"social_links"`
	BlueprintOrder *[]string `json:"blueprint_order"`
}

// UpdateAccount handles PATCH /api/v1/accounts/:account (admin only)
// Updates mutable account fields such as display_name.
func UpdateAccount(log *logger.Logger, accountStore *account.AccountStore, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req UpdateAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		displayName := ""
		displayNameMaxLength := accountDisplayNameMaxLength(acct.Type)
		if req.DisplayName != nil {
			displayName = strings.TrimSpace(*req.DisplayName)
			if acct.Type == "organization" && displayName == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "organization name can't be empty"})
				return
			}
			if displayName != "" && utf8.RuneCountInString(displayName) > displayNameMaxLength {
				c.JSON(http.StatusBadRequest, gin.H{"error": accountDisplayNameLengthError(acct.Type, displayNameMaxLength)})
				return
			}
		}
		if req.Bio != nil && len(*req.Bio) > 160 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bio must be 160 characters or fewer"})
			return
		}
		if req.Location != nil && len(*req.Location) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "location must be 100 characters or fewer"})
			return
		}
		if req.LocalTimezone != nil && len(*req.LocalTimezone) > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "local_timezone must be 50 characters or fewer"})
			return
		}
		if req.Pronouns != nil && len(*req.Pronouns) > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pronouns must be 50 characters or fewer"})
			return
		}
		if req.Website != nil {
			if len(*req.Website) > 255 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "website must be 255 characters or fewer"})
				return
			}
			if *req.Website != "" {
				u, err := url.ParseRequestURI(*req.Website)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
					c.JSON(http.StatusBadRequest, gin.H{"error": "website must be a valid http or https URL"})
					return
				}
			}
		}
		if req.SocialLinks != nil {
			if len(*req.SocialLinks) > 4 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "social_links may contain at most 4 entries"})
				return
			}
			for _, link := range *req.SocialLinks {
				if len(link) > 255 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "each social link must be 255 characters or fewer"})
					return
				}
			}
		}
		if req.BlueprintOrder != nil {
			if len(*req.BlueprintOrder) > 200 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "blueprint_order may contain at most 200 entries"})
				return
			}
		}

		if err := accountStore.UpdateProfile(acct.ID, displayName, req.Bio, req.Location, req.LocalTimezone, req.Pronouns, req.Website, req.SocialLinks, req.BlueprintOrder); err != nil {
			log.Error("accounts: update account profile failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
			return
		}

		log.Info("accounts: account profile updated", "account_id", acct.ID, "display_name", displayName)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.ProfileUpdate
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = acct.Name
		evt.Description = "Updated account profile"
		evt.Metadata = map[string]any{"display_name": displayName}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{"message": "profile updated"})
	}
}

// RenameAccountRequest represents the request body for renaming an account
type RenameAccountRequest struct {
	Name string `json:"name" binding:"required"`
}

// RenameAccount handles PUT /api/v1/accounts/:account (owner only)
func RenameAccount(log *logger.Logger, accountStore *account.AccountStore, agentIdx *agentindex.Index, avatarStore *avatar.Store, orgClient *org.Client, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RenameAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if err := accountStore.Rename(acct.ID, req.Name); err != nil {
			log.Error("accounts: rename account failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to rename account",
				"details": err.Error(),
			})
			return
		}

		// Sync the name to WorkOS for organization accounts
		if acct.WorkOSOrganizationID != "" && orgClient != nil {
			if err := orgClient.UpdateOrganizationName(c.Request.Context(), acct.WorkOSOrganizationID, req.Name); err != nil {
				log.Warn("accounts: update WorkOS organization name failed", "error", err, "account_id", acct.ID)
			}
		}

		// Move avatars in storage to match the new account name
		if avatarStore != nil && acct.Name != req.Name {
			agentNames, _ := agentIdx.AgentNames(acct.ID)
			if err := avatarStore.MoveAllForAccount(c.Request.Context(), acct.Name, req.Name, agentNames); err != nil {
				log.Warn("accounts: move avatars during rename failed", "error", err, "account_id", acct.ID)
			} else {
				// Stamp the moved avatars so their cache-busting tokens advance past
				// the old key's cached copy.
				if _, err := accountStore.TouchAvatarUpdatedAt(acct.ID); err != nil {
					log.Warn("accounts: stamp account avatar_updated_at after rename failed", "error", err, "account_id", acct.ID)
				}
				for _, name := range agentNames {
					if _, err := agentIdx.TouchAvatarUpdatedAt(acct.ID, name); err != nil {
						log.Warn("accounts: stamp agent avatar_updated_at after rename failed", "error", err, "account_id", acct.ID, "agent", name)
					}
				}
			}
		}

		log.Info("accounts: account renamed", "id", acct.ID, "old_name", acct.Name, "new_name", req.Name)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AccountRename
		evt.ResourceType = "account"
		evt.ResourceID = acct.ID
		evt.ResourceName = req.Name
		evt.Description = "Renamed account from " + acct.Name + " to " + req.Name
		evt.Metadata = map[string]any{
			"before": map[string]any{"name": acct.Name},
			"after":  map[string]any{"name": req.Name},
		}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{
			"message": "account renamed",
			"name":    req.Name,
		})
	}
}

// UpdateProfileRequest represents the request body for updating user profile
type UpdateProfileRequest struct {
	DisplayName string `json:"display_name"`
}

// UpdateProfile handles PATCH /api/v1/me (protected)
// Updates the display name on the user's personal account.
func UpdateProfile(log *logger.Logger, accountStore *account.AccountStore, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var req UpdateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.DisplayName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "display name is required"})
			return
		}
		if utf8.RuneCountInString(req.DisplayName) > account.DisplayNameMaxLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": accountDisplayNameLengthError("personal", account.DisplayNameMaxLength)})
			return
		}

		// Find the user's personal account
		accounts, err := accountStore.GetAccountsForUser(user.ID)
		if err != nil {
			log.Error("accounts: fetch accounts failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch accounts"})
			return
		}

		var personalAccountID string
		for _, a := range accounts {
			if a.Type == "personal" {
				personalAccountID = a.ID
				break
			}
		}
		if personalAccountID == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "personal account not found"})
			return
		}

		if err := accountStore.UpdateDisplayName(personalAccountID, req.DisplayName); err != nil {
			log.Error("accounts: update display name failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update display name"})
			return
		}

		log.Info("accounts: display name updated", "user_id", user.ID, "account_id", personalAccountID)

		evt := auditlog.FromGinContext(c, personalAccountID)
		evt.Action = auditlog.ProfileUpdate
		evt.ResourceType = "account"
		evt.ResourceID = personalAccountID
		evt.Description = "Updated profile display name"
		evt.Metadata = map[string]any{"display_name": req.DisplayName}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, UpdateProfileResponse{
			User: &ProfileUser{
				ID:    user.ID,
				Email: user.Email,
			},
		})
	}
}

// GetProfile handles GET /api/v1/me (protected)
func GetProfile(log *logger.Logger, accountStore *account.AccountStore, agentIndex *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accounts, err := accountStore.GetAccountsForUser(user.ID)
		if err != nil {
			log.Error("accounts: get accounts for user failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get accounts"})
			return
		}

		accountResponses := make([]AccountWithRoleResponse, 0, len(accounts))
		for _, a := range accounts {
			resp := AccountWithRoleResponse{
				ID:                   a.ID,
				Name:                 a.Name,
				Type:                 a.Type,
				DisplayName:          a.DisplayName,
				WorkOSOrganizationID: a.WorkOSOrganizationID,
			}

			// Include agent summaries for each account
			page, err := agentIndex.ListForAccount(a.ID, agentindex.BlueprintListOptions{})
			if err == nil {
				summaries := make([]AgentSummary, 0, len(page.Agents))
				for _, agent := range page.Agents {
					summaries = append(summaries, AgentSummary{
						Name:       agent.Name,
						Registry:   agent.Registry,
						Visibility: agent.Visibility,
						BuildCount: agent.VersionCount,
					})
				}
				resp.Agents = summaries
			}

			accountResponses = append(accountResponses, resp)
		}

		c.JSON(http.StatusOK, ProfileResponse{
			User: &ProfileUser{
				ID:    user.ID,
				Email: user.Email,
			},
			Accounts: accountResponses,
		})
	}
}

// CheckAccountName handles GET /api/v1/accounts/check/:name (public)
// Returns whether an account name is available
func CheckAccountName(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		// Validate name format and restrictions
		if err := account.ValidateAccountName(name); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"available": false,
				"reason":    err.Error(),
			})
			return
		}
		if err := account.CheckAccountNameRestricted(name); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"available": false,
				"reason":    err.Error(),
			})
			return
		}

		// Check if name is taken
		_, err := accountStore.GetByName(name)
		if err != nil {
			// Not found = available
			c.JSON(http.StatusOK, gin.H{"available": true})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"reason":    "name is already taken",
		})
	}
}

// SearchAccountResult represents a single account in search results
type SearchAccountResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// SearchAccounts handles GET /api/v1/accounts/search (protected)
// Query params: q (required), type (optional: personal|organization), limit (optional, default 10, max 10)
func SearchAccounts(log *logger.Logger, accountStore *account.AccountStore, avatarStore *avatar.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := strings.ToLower(strings.TrimSpace(c.Query("q")))
		if len(q) < 3 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' must be at least 3 characters"})
			return
		}

		accountType := c.Query("type")
		if accountType != "" && accountType != "personal" && accountType != "organization" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'personal' or 'organization'"})
			return
		}

		limit := 10
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 10 {
				limit = parsed
			}
		}

		accounts, err := accountStore.Search(q, accountType, limit)
		if err != nil {
			log.Error("accounts: search accounts failed", "error", err, "query", q)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search accounts"})
			return
		}

		results := make([]SearchAccountResult, 0, len(accounts))
		for _, a := range accounts {
			result := SearchAccountResult{
				ID:          a.ID,
				Name:        a.Name,
				DisplayName: a.DisplayName,
				Type:        a.Type,
			}
			if avatarStore != nil {
				result.AvatarURL = avatarStore.AvatarURL(a.Name, a.AvatarUpdatedAt)
			}
			results = append(results, result)
		}

		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// GetAccountOrgs handles GET /api/v1/accounts/:account/orgs (public)
// Returns the organization accounts the owner of the given personal account belongs to.
// Intentionally unauthenticated: org memberships are visible on the public profile page.
// TODO: when org accounts have a visibility field, filter here — authenticated owner gets all orgs,
// unauthenticated visitors get public orgs only.
func GetAccountOrgs(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		if acct.Type != "personal" {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		ownerID, err := accountStore.GetFirstMemberUserID(acct.ID)
		if err != nil {
			// No members — return empty list
			c.JSON(http.StatusOK, gin.H{"orgs": []AccountOrgResponse{}})
			return
		}

		orgs, err := accountStore.GetOrgAccountsForUser(ownerID)
		if err != nil {
			log.Error("accounts: get org accounts failed", "error", err, "account", accountName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get org accounts"})
			return
		}

		resp := make([]AccountOrgResponse, 0, len(orgs))
		for _, o := range orgs {
			resp = append(resp, AccountOrgResponse{
				Name:         o.Name,
				DisplayName:  o.DisplayName,
				AvatarColors: o.AvatarColors,
			})
		}

		c.JSON(http.StatusOK, gin.H{"orgs": resp})
	}
}
