package handlers

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"database/sql"

	spec "github.com/astropods/astro-spec"
	"github.com/astropods/astro/apps/astro-server/internal/arn"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type encryptedCredentials struct {
	Credentials []knowledgestore.Credential
	DataKey     []byte
	KMSKeyARN   string
}

func encryptKnowledgeCreds(ctx context.Context, log *logger.Logger, vault *envelope.Vault, creds map[string]string) (*encryptedCredentials, error) {
	enc, err := vault.Encryptor(ctx)
	if err != nil {
		log.Error("knowledge: create KMS encryptor failed", "error", err)
		return nil, fmt.Errorf("failed to encrypt credentials")
	}
	encrypted, err := knowledgestore.EncryptCredentials(enc, creds)
	if err != nil {
		log.Error("knowledge: encrypt credentials failed", "error", err)
		return nil, fmt.Errorf("failed to encrypt credentials")
	}
	return &encryptedCredentials{
		Credentials: encrypted,
		DataKey:     enc.EncryptedDataKey,
		KMSKeyARN:   enc.KMSKeyARN,
	}, nil
}

// KnowledgeEndpointResponse is the API representation of a PrivateLink endpoint.
type KnowledgeEndpointResponse struct {
	CloudProvider   string  `json:"cloud_provider"`
	EndpointService string  `json:"endpoint_service"`
	Region          string  `json:"region"`
	EndpointID      *string `json:"endpoint_id,omitempty"`
	EndpointDNS     *string `json:"endpoint_dns,omitempty"`
	Status          string  `json:"status"`
	Error           *string `json:"error,omitempty"`
}

// KnowledgeResponse is the API representation of a knowledge store.
type KnowledgeResponse struct {
	AccountID string `json:"account_id,omitempty"`
	Account   string `json:"account,omitempty"`
	ID        string `json:"id"`
	ARN       string `json:"arn"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	// Mode is always "external" for stores created now. Rows predating the
	// removal of platform-provisioned stores still report "managed".
	Mode        string                      `json:"mode"`
	Status      string                      `json:"status"`
	Endpoint    *KnowledgeEndpointResponse  `json:"endpoint,omitempty"`
	Error       *string                     `json:"error,omitempty"`
	Annotations map[string]string           `json:"annotations,omitempty"`
	CreatedAt   time.Time                   `json:"created_at"`
	UpdatedAt   time.Time                   `json:"updated_at"`
	BoundAgents []knowledgestore.BoundAgent `json:"bound_agents,omitempty"`
}

// KnowledgeCredentialsResponse holds decrypted credentials for a knowledge store.
type KnowledgeCredentialsResponse map[string]string

// parseRegionFromServiceName extracts the AWS region from a VPC endpoint
// service name like "com.amazonaws.vpce.us-east-1.vpce-svc-0123456789abcdef".
func parseRegionFromServiceName(service string) string {
	parts := strings.Split(service, ".")
	if len(parts) < 5 || parts[0] != "com" || parts[1] != "amazonaws" || parts[2] != "vpce" {
		return ""
	}
	return parts[3]
}

func toKnowledgeResponse(ks *knowledgestore.KnowledgeStore) KnowledgeResponse {
	return KnowledgeResponse{
		AccountID:   ks.AccountID,
		ID:          ks.ID,
		ARN:         ks.ARN,
		Name:        ks.Name,
		Provider:    ks.Provider,
		Mode:        ks.Mode,
		Status:      ks.Status,
		Error:       ks.Error,
		Annotations: ks.Annotations,
		CreatedAt:   ks.CreatedAt,
		UpdatedAt:   ks.UpdatedAt,
	}
}

// ConnectKnowledgeStore onboards an external (bring-your-own) database under an ARN.
// No K8s resources are created — the platform is a credential broker only.
func ConnectKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, pipesClient *pipes.Client, cfg *config.Config, vault *envelope.Vault, queue *riverqueue.Queue, db *sql.DB, quotaCheck quota.Checker) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		var req struct {
			Name            string `json:"name" binding:"required"`
			Provider        string `json:"provider" binding:"required"`
			Host            string `json:"host" binding:"required"`
			Port            int    `json:"port" binding:"required"`
			Database        string `json:"database"`
			Username        string `json:"username"`
			Password        string `json:"password"`
			APIKey          string `json:"api_key"`
			SkipHealthCheck bool   `json:"skip_health_check"`
			PrivateLink     bool   `json:"private_link"`
			// SupabaseProject, when set, marks this store as imported from Supabase.
			// The server composes the store annotations from it (the store itself is
			// created as plain "postgres", so this is the only record of the origin).
			SupabaseProject *struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				Region         string `json:"region"`
				OrganizationID string `json:"organization_id"`
			} `json:"supabase_project"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Compose origin annotations server-side (the backend owns the schema).
		var annotations knowledgestore.Annotations
		if req.SupabaseProject != nil && req.SupabaseProject.ID != "" {
			annotations = knowledgestore.Annotations{
				"source":                "supabase",
				"supabase_project_id":   req.SupabaseProject.ID,
				"supabase_project_name": req.SupabaseProject.Name,
				"region":                req.SupabaseProject.Region,
				"organization_id":       req.SupabaseProject.OrganizationID,
			}
		}

		if err := knowledgestore.ValidateStoreName(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if _, ok := spec.LookupBuiltin("knowledge", req.Provider); !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported provider: %s", req.Provider)})
			return
		}

		// Build credential map from request fields.
		creds := map[string]string{
			"HOST": req.Host,
			"PORT": strconv.Itoa(req.Port),
		}
		if req.Database != "" {
			creds["DATABASE"] = req.Database
		}
		if req.Username != "" {
			creds["USERNAME"] = req.Username
		}
		if req.Password != "" {
			creds["PASSWORD"] = req.Password
		}
		if req.APIKey != "" {
			creds["API_KEY"] = req.APIKey
		}

		// For Supabase-imported stores, connect through the session pooler rather
		// than the direct db.<ref>.supabase.co endpoint the client sends. The direct
		// endpoint is IPv6-only and unreachable from our IPv4-only VPCs; the pooler
		// is IPv4. The pooler host (aws-0 vs aws-1 cluster) and user (postgres.<ref>)
		// aren't derivable from the region, so we resolve them from the Management
		// API and override the credentials — the backend owns the connection shape.
		if req.SupabaseProject != nil && req.SupabaseProject.ID != "" {
			if !supabaseProjectRefPattern.MatchString(req.SupabaseProject.ID) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supabase project ref"})
				return
			}
			session, ok := middleware.GetSession(c)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
				return
			}
			overlay, err := supabasePoolerOverlay(c.Request.Context(), pipesClient, session.UserID, session.OrganizationID, req.SupabaseProject.ID)
			if err != nil {
				respondSupabaseResolveError(c, log, err)
				return
			}
			maps.Copy(creds, overlay)
		}

		if err := knowledgestore.ValidateExternalCredentials(req.Provider, creds); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		storeID := deployid.New()
		storeARN := arn.KnowledgeStore(acct.ID, req.Name)

		enc, err := encryptKnowledgeCreds(c.Request.Context(), log, vault, creds)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var encryptedDataKey []byte
		var kmsKeyARN string
		if enc != nil {
			encryptedDataKey = enc.DataKey
			kmsKeyARN = enc.KMSKeyARN
		}

		ks, err := ksStore.Create(knowledgestore.CreateParams{
			ID:               storeID,
			AccountID:        acct.ID,
			Name:             req.Name,
			ARN:              storeARN,
			Provider:         req.Provider,
			Status:           knowledgestore.StatusReady,
			EncryptedDataKey: encryptedDataKey,
			KMSKeyARN:        kmsKeyARN,
			Annotations:      annotations,
		})
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				c.JSON(http.StatusConflict, gin.H{"error": "a knowledge store with this name already exists"})
				return
			}
			log.Error("knowledge: create external knowledge store record failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create store"})
			return
		}

		if enc != nil {
			if err := ksStore.SaveCredentials(storeID, enc.Credentials); err != nil {
				log.Error("knowledge: save credentials failed", "error", err, "store_id", storeID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save credentials"})
				return
			}
		}

		// If PrivateLink is requested, create the endpoint and enqueue provisioning.
		// The health check is deferred — the DB won't be reachable until the
		// VPC endpoint is accepted and DNS propagates.
		if req.PrivateLink {
			if quotaCheck != nil {
				if res, err := quotaCheck.Check(c.Request.Context(), acct.ID, quota.ResourceKnowledgeEndpoints); err == nil && res.Blocked {
					c.JSON(http.StatusPaymentRequired, quota.LimitResponse(res))
					return
				}
			}
			if cfg.Deployment.PrivateLinkVpcID == "" {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PrivateLink is not configured on this platform"})
				return
			}
			if !strings.HasPrefix(req.Host, "com.amazonaws.vpce.") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "host must be an AWS VPC endpoint service name (com.amazonaws.vpce.<region>.vpce-svc-...) when using --private-link"})
				return
			}
			region := parseRegionFromServiceName(req.Host)
			if region == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "could not parse AWS region from host; expected format: com.amazonaws.vpce.<region>.vpce-svc-..."})
				return
			}

			if _, epErr := ksStore.CreateEndpoint(knowledgestore.EndpointParams{
				KnowledgeStoreID: storeID,
				CloudProvider:    "aws",
				EndpointService:  req.Host,
				Region:           region,
			}); epErr != nil {
				log.Error("knowledge: create endpoint record failed", "error", epErr, "store_id", storeID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create endpoint"})
				return
			}
			if err := ksStore.SetStatus(storeID, knowledgestore.StatusConnecting); err != nil {
				log.Error("knowledge: update store status to connecting failed", "error", err, "store_id", storeID)
			}
			if err := queue.InsertPrivateLinkProvisionJob(c.Request.Context(), storeID); err != nil {
				log.Error("knowledge: enqueue PrivateLink provision job failed", "error", err, "store_id", storeID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue provision job"})
				return
			}

			// Re-read so the response reflects the connecting status.
			ks, _ = ksStore.GetByID(storeID)
			log.Info("knowledge: external knowledge store connected with PrivateLink", "store_id", storeID, "provider", req.Provider, "arn", storeARN, "region", region)

			c.JSON(http.StatusOK, toKnowledgeResponse(ks))
			return
		}

		// Run a connectivity health check unless explicitly skipped.
		if !req.SkipHealthCheck {
			hctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()
			if hErr := knowledgestore.CheckHealth(hctx, req.Provider, creds); hErr != nil {
				msg := knowledgestore.HumanizeHealthCheckError(hErr)
				log.Warn("knowledge: external knowledge store health check failed", "store_id", storeID, "provider", req.Provider, "error", hErr)
				if sErr := ksStore.SetError(storeID, msg); sErr != nil {
					log.Error("knowledge: set error status after health check failure failed", "error", sErr, "store_id", storeID)
				}
				// Re-read the store so the response reflects the error status.
				ks, _ = ksStore.GetByID(storeID)
			}
		}

		log.Info("knowledge: external knowledge store connected", "store_id", storeID, "provider", req.Provider, "arn", storeARN)

		c.JSON(http.StatusOK, toKnowledgeResponse(ks))
	}
}

// UpdateKnowledgeStoreCredentials updates the connection credentials of an
// existing connected store. Only the provided fields change; the rest are
// preserved. The merged credentials are health-checked before being persisted,
// so a bad update can't leave the store with unusable credentials.
// For Supabase-imported stores, HOST/PORT/USERNAME are server-managed (resolved
// from the session pooler) and cannot be edited here.
func UpdateKnowledgeStoreCredentials(log *logger.Logger, ksStore *knowledgestore.Store, pipesClient *pipes.Client, cfg *config.Config, vault *envelope.Vault) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil {
			log.Error("knowledge: get knowledge store failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get store"})
			return
		}
		if ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}
		var req struct {
			Host            *string `json:"host"`
			Port            *int    `json:"port"`
			Database        *string `json:"database"`
			Username        *string `json:"username"`
			Password        *string `json:"password"`
			APIKey          *string `json:"api_key"`
			SkipHealthCheck bool    `json:"skip_health_check"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Collect the requested changes. Supabase-managed connection coordinates
		// are rejected up front so the caller gets a clear error (validated before
		// any decryption so these checks stay cheap and testable).
		isSupabase := ks.Annotations["source"] == "supabase"
		updates := map[string]string{}
		if req.Host != nil {
			if isSupabase {
				c.JSON(http.StatusBadRequest, gin.H{"error": "host is managed by Supabase and cannot be changed"})
				return
			}
			updates["HOST"] = *req.Host
		}
		if req.Port != nil {
			if isSupabase {
				c.JSON(http.StatusBadRequest, gin.H{"error": "port is managed by Supabase and cannot be changed"})
				return
			}
			updates["PORT"] = strconv.Itoa(*req.Port)
		}
		if req.Username != nil {
			if isSupabase {
				c.JSON(http.StatusBadRequest, gin.H{"error": "username is managed by Supabase and cannot be changed"})
				return
			}
			updates["USERNAME"] = *req.Username
		}
		if req.Database != nil {
			updates["DATABASE"] = *req.Database
		}
		if req.Password != nil {
			updates["PASSWORD"] = *req.Password
		}
		if req.APIKey != nil {
			updates["API_KEY"] = *req.APIKey
		}

		// Supabase stores' connection coordinates are server-managed: re-resolve
		// them from the session pooler so an update also repairs stores created
		// before the pooler migration (stale direct host / bare "postgres" user).
		// This overrides the stored HOST/PORT/USERNAME regardless of which fields
		// the caller changed.
		if isSupabase {
			ref := ks.Annotations["supabase_project_id"]
			if !supabaseProjectRefPattern.MatchString(ref) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "store is missing a valid Supabase project reference"})
				return
			}
			session, ok := middleware.GetSession(c)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
				return
			}
			overlay, err := supabasePoolerOverlay(c.Request.Context(), pipesClient, session.UserID, session.OrganizationID, ref)
			if err != nil {
				respondSupabaseResolveError(c, log, err)
				return
			}
			// Don't clobber a user-provided database.
			if _, ok := updates["DATABASE"]; ok {
				delete(overlay, "DATABASE")
			}
			maps.Copy(updates, overlay)
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no credential fields provided"})
			return
		}

		if len(ks.EncryptedDataKey) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "store has no stored credentials to update"})
			return
		}

		dbCreds, err := ksStore.GetCredentials(ks.ID)
		if err != nil {
			log.Error("knowledge: get credentials failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve credentials"})
			return
		}
		merged, err := knowledgestore.ResolveCredentials(c.Request.Context(), ks, dbCreds, vault)
		if err != nil {
			log.Error("knowledge: resolve credentials failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve credentials"})
			return
		}
		maps.Copy(merged, updates)

		if err := knowledgestore.ValidateExternalCredentials(ks.Provider, merged); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Health-check the merged credentials before persisting. PrivateLink
		// stores are skipped: their host is only reachable in-cluster and the
		// check would spuriously fail (mirrors the connect flow).
		ep, _ := ksStore.GetEndpoint(ks.ID)
		if !req.SkipHealthCheck && ep == nil {
			hctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()
			if hErr := knowledgestore.CheckHealth(hctx, ks.Provider, merged); hErr != nil {
				log.Warn("knowledge: store credential update health check failed", "store_id", ks.ID, "provider", ks.Provider, "error", hErr)
				c.JSON(http.StatusBadRequest, gin.H{"error": knowledgestore.HumanizeHealthCheckError(hErr)})
				return
			}
		}

		// Persist only the changed keys, re-encrypted under the store's existing
		// data key.
		if err := ksStore.RewriteCredentials(c.Request.Context(), vault, ks, updates); err != nil {
			log.Error("knowledge: update credentials failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update credentials"})
			return
		}

		// The credentials now work (or the check was skipped); clear any prior
		// error and mark the store ready.
		if err := ksStore.SetStatus(ks.ID, knowledgestore.StatusReady); err != nil {
			log.Error("knowledge: mark store ready failed", "error", err, "store_id", ks.ID)
		}

		ks, _ = ksStore.GetByID(ks.ID)
		log.Info("knowledge: store credentials updated", "store_id", ks.ID, "provider", ks.Provider, "fields", len(updates))
		c.JSON(http.StatusOK, toKnowledgeResponse(ks))
	}
}

func ListKnowledgeStores(log *logger.Logger, ksStore *knowledgestore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		stores, err := ksStore.ListByAccount(acct.ID)
		if err != nil {
			log.Error("knowledge: list knowledge stores failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stores"})
			return
		}

		resp := make([]KnowledgeResponse, 0, len(stores))
		for _, s := range stores {
			resp = append(resp, toKnowledgeResponse(s))
		}
		c.JSON(http.StatusOK, resp)
	}
}

func GetKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil {
			log.Error("knowledge: get knowledge store failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get store"})
			return
		}
		if ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}

		resp := toKnowledgeResponse(ks)

		// Include bound agents.
		if agents, bindErr := ksStore.GetBoundAgents(c.Request.Context(), ks.ID); bindErr == nil && len(agents) > 0 {
			resp.BoundAgents = agents
		}

		// Include PrivateLink endpoint info if present.
		if ep, epErr := ksStore.GetEndpoint(ks.ID); epErr == nil && ep != nil {
			resp.Endpoint = &KnowledgeEndpointResponse{
				CloudProvider:   ep.CloudProvider,
				EndpointService: ep.EndpointService,
				Region:          ep.Region,
				EndpointID:      ep.EndpointID,
				EndpointDNS:     ep.EndpointDNS,
				Status:          ep.Status,
				Error:           ep.Error,
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// RecheckKnowledgeStore re-resolves a PrivateLink store's VPC endpoint and
// rewrites its HOST credential to the live endpoint DNS. It exists to repair
// stores whose HOST still holds the original "com.amazonaws.vpce.*" service
// name — the reconciler only rewrites HOST on the available transition, so
// stores that became ready before that logic existed never get corrected.
func RecheckKnowledgeStore(
	log *logger.Logger,
	ksStore *knowledgestore.Store,
	newEC2 func(context.Context) (knowledgestore.EC2Client, error),
	vault *envelope.Vault,
) gin.HandlerFunc {
	if newEC2 == nil {
		newEC2 = knowledgestore.NewEC2Client
	}
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil {
			log.Error("knowledge: get knowledge store failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get store"})
			return
		}
		if ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}
		ep, err := ksStore.GetEndpoint(ks.ID)
		if err != nil {
			log.Error("knowledge: get endpoint failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get endpoint"})
			return
		}
		if ep == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "store has no PrivateLink endpoint to recheck"})
			return
		}
		if ep.EndpointID == nil || *ep.EndpointID == "" {
			c.JSON(http.StatusConflict, gin.H{"error": "PrivateLink endpoint is still being provisioned; try again shortly"})
			return
		}

		// Re-resolve the live endpoint DNS from AWS; fall back to the
		// reconciler-stored DNS if the AWS lookup is unavailable.
		dns := resolveEndpointDNS(c.Request.Context(), log, newEC2, *ep.EndpointID)
		if dns == "" && ep.EndpointDNS != nil {
			dns = *ep.EndpointDNS
		}
		if dns == "" {
			c.JSON(http.StatusConflict, gin.H{"error": "endpoint DNS is not available yet; try again shortly"})
			return
		}

		// Persist the resolved DNS on the endpoint record and rewrite the
		// HOST credential to it.
		if err := ksStore.SetEndpointReady(ks.ID, *ep.EndpointID, dns); err != nil {
			log.Error("knowledge: record endpoint DNS failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update endpoint"})
			return
		}
		if err := ksStore.RewriteHostCredential(c.Request.Context(), vault, ks, dns); err != nil {
			log.Error("knowledge: rewrite host credential failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update host"})
			return
		}
		if ks.Status != knowledgestore.StatusReady {
			if err := ksStore.SetStatus(ks.ID, knowledgestore.StatusReady); err != nil {
				log.Error("knowledge: mark store ready failed", "error", err, "store_id", ks.ID)
			}
			ks.Status = knowledgestore.StatusReady
		}

		resp := toKnowledgeResponse(ks)
		ep.EndpointDNS = &dns
		ep.Status = knowledgestore.StatusReady
		resp.Endpoint = &KnowledgeEndpointResponse{
			CloudProvider:   ep.CloudProvider,
			EndpointService: ep.EndpointService,
			Region:          ep.Region,
			EndpointID:      ep.EndpointID,
			EndpointDNS:     ep.EndpointDNS,
			Status:          ep.Status,
			Error:           ep.Error,
		}
		c.JSON(http.StatusOK, resp)
	}
}

// resolveEndpointDNS returns the live primary DNS name for a VPC endpoint, or
// "" if it can't be resolved (endpoint not available, no DNS yet, AWS error —
// all non-fatal; the caller falls back to the stored value).
func resolveEndpointDNS(ctx context.Context, log *logger.Logger, newEC2 func(context.Context) (knowledgestore.EC2Client, error), endpointID string) string {
	ec2Client, err := newEC2(ctx)
	if err != nil {
		log.Warn("recheck: create EC2 client, using stored DNS failed", "error", err)
		return ""
	}
	out, err := ec2Client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
		VpcEndpointIds: []string{endpointID},
	})
	if err != nil || out == nil || len(out.VpcEndpoints) == 0 {
		log.Warn("recheck: describe VPC endpoint, using stored DNS failed", "error", err, "vpce_id", endpointID)
		return ""
	}
	vpce := out.VpcEndpoints[0]
	if strings.ToLower(string(vpce.State)) != "available" || len(vpce.DnsEntries) == 0 {
		return ""
	}
	return aws.ToString(vpce.DnsEntries[0].DnsName)
}

func DeleteKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, queue *riverqueue.Queue) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil {
			log.Error("knowledge: get knowledge store failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get store"})
			return
		}
		if ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}

		// Refuse to delete a store that has active deployment bindings.
		bound, err := ksStore.GetBoundAgents(c.Request.Context(), ks.ID)
		if err != nil {
			log.Error("knowledge: check store bindings failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check store bindings"})
			return
		}
		if len(bound) > 0 {
			c.JSON(http.StatusConflict, gin.H{
				"error":       "store has active deployment bindings",
				"deployments": bound,
			})
			return
		}

		// Enqueue PrivateLink cleanup if the store has an endpoint.
		// Capture the VPCE ID before the DB row is cascade-deleted.
		if ep, epErr := ksStore.GetEndpoint(ks.ID); epErr == nil && ep != nil && queue != nil {
			endpointID := ""
			if ep.EndpointID != nil {
				endpointID = *ep.EndpointID
			}
			if err := queue.InsertPrivateLinkDeleteJob(c.Request.Context(), ks.ID, endpointID); err != nil {
				log.Error("knowledge: enqueue PrivateLink delete job failed", "error", err, "store_id", ks.ID)
			}
		}

		if err := ksStore.Delete(ks.ID); err != nil {
			log.Error("knowledge: delete knowledge store record failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

// GetKnowledgeStoreCredentials decrypts and returns the store's connection
// credentials.
func GetKnowledgeStoreCredentials(log *logger.Logger, ksStore *knowledgestore.Store, vault *envelope.Vault) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil || ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}

		creds, err := ksStore.GetCredentials(ks.ID)
		if err != nil {
			log.Error("knowledge: get credentials failed", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve credentials"})
			return
		}

		plainCreds, resolveErr := knowledgestore.ResolveCredentials(c.Request.Context(), ks, creds, vault)
		if resolveErr != nil {
			log.Error("knowledge: resolve credentials failed", "error", resolveErr, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve credentials"})
			return
		}

		c.JSON(http.StatusOK, plainCreds)
	}
}
