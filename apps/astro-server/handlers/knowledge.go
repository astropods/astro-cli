package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"database/sql"

	"github.com/astropods/astro/apps/astro-server/internal/arn"
	"github.com/astropods/astro/apps/astro-server/internal/clustercfg"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// encryptedCredentials holds the result of encrypting credentials with KMS.
type encryptedCredentials struct {
	Credentials []knowledgestore.Credential
	DataKey     []byte
	KMSKeyARN   string
}

// encryptKnowledgeCreds encrypts a set of plaintext credentials using KMS envelope
// encryption. Returns nil if kmsKeyARN is empty (KMS not configured).
func encryptKnowledgeCreds(ctx context.Context, log *logger.Logger, kmsKeyARN string, creds map[string]string) (*encryptedCredentials, error) {
	if kmsKeyARN == "" {
		return nil, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Error("Failed to load AWS config", "error", err)
		return nil, fmt.Errorf("failed to initialize KMS")
	}
	kmsClient := kms.NewFromConfig(awsCfg)
	enc, err := envelope.NewEncryptor(ctx, kmsClient, kmsKeyARN)
	if err != nil {
		log.Error("Failed to create KMS encryptor", "error", err)
		return nil, fmt.Errorf("failed to encrypt credentials")
	}
	encrypted, err := knowledgestore.EncryptCredentials(enc, creds)
	if err != nil {
		log.Error("Failed to encrypt credentials", "error", err)
		return nil, fmt.Errorf("failed to encrypt credentials")
	}
	return &encryptedCredentials{
		Credentials: encrypted,
		DataKey:     enc.EncryptedDataKey,
		KMSKeyARN:   kmsKeyARN,
	}, nil
}

// KnowledgeEvent is a single provisioning event for a knowledge store.
type KnowledgeEvent struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Count   int32  `json:"count"`
}

// humanizeKnowledgeEvent translates raw K8s event reason+message into
// user-friendly text. Unknown reasons pass through unchanged.
func humanizeKnowledgeEvent(reason, message string) (string, string) {
	switch reason {
	case "Scheduled":
		return "Assigning resources", "Infrastructure is being allocated for your store."
	case "SuccessfulAttachVolume":
		return "Storage attached", "Persistent storage has been connected."
	case "Pulling":
		return "Downloading database engine", "Fetching the database image — this may take a moment."
	case "Pulled":
		return "Database engine ready", "The database image is downloaded and ready."
	case "Created":
		return "Preparing store", "Your store container has been created."
	case "Started":
		return "Store starting up", "Your store is booting and will be ready shortly."
	case "Unhealthy":
		return "Health check pending", "The store is still initializing — waiting for it to become healthy."
	case "BackOff":
		return "Retrying", "A transient issue occurred and the system is retrying automatically."
	case "FailedScheduling":
		return "Waiting for resources", "Waiting for infrastructure capacity to become available."
	case "FailedMount", "FailedAttachVolume":
		return "Storage issue", "There was a problem attaching storage — the system will retry."
	default:
		return reason, message
	}
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
	ID           string                      `json:"id"`
	ARN          string                      `json:"arn"`
	Name         string                      `json:"name"`
	Provider     string                      `json:"provider"`
	Mode         string                      `json:"mode"`
	Status       string                      `json:"status"`
	Storage      string                      `json:"storage"`
	StorageClass *string                     `json:"storage_class,omitempty"`
	Public       bool                        `json:"public"`
	PublicHost   *string                     `json:"public_host,omitempty"`
	Endpoint     *KnowledgeEndpointResponse  `json:"endpoint,omitempty"`
	Error        *string                     `json:"error,omitempty"`
	CreatedAt    time.Time                   `json:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at"`
	Events       []KnowledgeEvent            `json:"events,omitempty"`
	BoundAgents  []knowledgestore.BoundAgent `json:"bound_agents,omitempty"`
}

// KnowledgeCredentialsResponse holds decrypted credentials for a knowledge store.
type KnowledgeCredentialsResponse map[string]string

// KnowledgeMetricsResponse holds infrastructure metrics for a knowledge store.
type KnowledgeMetricsResponse struct {
	CPUCores      *float64 `json:"cpu_cores"`      // Current CPU usage in cores
	MemoryBytes   *int64   `json:"memory_bytes"`   // Current memory working set in bytes
	StorageUsed   *int64   `json:"storage_used"`   // PVC bytes used
	StorageTotal  *int64   `json:"storage_total"`  // PVC bytes capacity
	UptimeSeconds int64    `json:"uptime_seconds"` // Seconds since store was created
}

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
		ID:           ks.ID,
		ARN:          ks.ARN,
		Name:         ks.Name,
		Provider:     ks.Provider,
		Mode:         ks.Mode,
		Status:       ks.Status,
		Storage:      ks.Storage,
		StorageClass: ks.StorageClass,
		Public:       ks.Public,
		PublicHost:   ks.PublicHost,
		Error:        ks.Error,
		CreatedAt:    ks.CreatedAt,
		UpdatedAt:    ks.UpdatedAt,
	}
}

func CreateKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, k8sReg *k8s.Registry, cfg *config.Config, omClient *openmeter.Client, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		var req struct {
			Name         string `json:"name" binding:"required"`
			Provider     string `json:"provider" binding:"required"`
			Storage      string `json:"storage"`
			StorageClass string `json:"storage_class"`
			Public       bool   `json:"public"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		storage := req.Storage
		if storage == "" {
			storage = "10Gi"
		}

		if err := knowledgestore.ValidateStoreName(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := knowledgestore.ValidateStorageSize(storage); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if _, ok := spec.LookupBuiltin("knowledge", req.Provider); !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported provider: %s", req.Provider)})
			return
		}

		if !cfg.Deployment.KnowledgeAllowManagedCreate {
			c.JSON(http.StatusForbidden, gin.H{"error": "managed knowledge store creation is disabled; connect your existing database via POST /api/v1/accounts/{account}/knowledge/connect"})
			return
		}

		storeID := deployid.New()
		storeARN := arn.KnowledgeStore(acct.ID, req.Name)

		clusterID := ""
		if acct.ClusterID != nil {
			clusterID = *acct.ClusterID
		}
		ingressCfg, ingressErr := clustercfg.Resolve(c.Request.Context(), k8sReg, cfg.Deployment, clusterID)
		if ingressErr != nil {
			log.Error("Failed to resolve cluster ingress config", "error", ingressErr, "cluster_id", clusterID)
			c.JSON(http.StatusBadRequest, gin.H{"error": ingressErr.Error()})
			return
		}
		var publicHost string
		if req.Public && ingressCfg.KnowledgeDomain != "" {
			publicHost = fmt.Sprintf("%s.%s.%s", req.Name, acct.Name, ingressCfg.KnowledgeDomain)
		}


		plainCreds, err := knowledgestore.GenerateCredentials(req.Provider)
		if err != nil {
			log.Error("Failed to generate credentials", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate credentials"})
			return
		}

		enc, err := encryptKnowledgeCreds(c.Request.Context(), log, cfg.Deployment.KMSKeyARN, plainCreds)
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
			Storage:          storage,
			StorageClass:     req.StorageClass,
			Public:           req.Public,
			PublicHost:       publicHost,
			EncryptedDataKey: encryptedDataKey,
			KMSKeyARN:        kmsKeyARN,
		})
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				c.JSON(http.StatusConflict, gin.H{"error": "a knowledge store with this name already exists"})
				return
			}
			log.Error("Failed to create knowledge store record", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create store"})
			return
		}

		if enc != nil {
			if err := ksStore.SaveCredentials(storeID, enc.Credentials); err != nil {
				// Non-fatal — reconciler will not be able to recover the secret, but the
				// K8s secret created in provisionStoreAsync will still work until cluster migration.
				log.Error("Failed to save credentials", "error", err, "store_id", storeID)
			}
		}

		if k8sClient != nil {
			go provisionStoreAsync(context.Background(), log, ksStore, k8sClient, ks, plainCreds, cfg)
		}

		// Emit metering events (fire-and-forget) so entitlement checks see the new store immediately.
		go openmeter.EmitActiveKnowledgeStores(context.Background(), omClient, db, log, acct.ID)
		go openmeter.EmitKnowledgeStorage(context.Background(), omClient, db, log, acct.ID)

		c.JSON(http.StatusAccepted, toKnowledgeResponse(ks))
	}
}

// provisionStoreAsync creates K8s resources after the 202 is sent. Uses context.Background()
// intentionally — the request context is already cancelled by the time this runs.
func provisionStoreAsync(ctx context.Context, log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, ks *knowledgestore.KnowledgeStore, plainCreds map[string]string, cfg *config.Config) {
	secretName := k8s.KnowledgeSecretName(ks.ID)

	if err := k8s.EnsureKnowledgeNamespace(ctx, k8sClient, ks.AccountID); err != nil {
		log.Error("Failed to ensure knowledge namespace", "error", err, "store_id", ks.ID)
		if setErr := ksStore.SetError(ks.ID, "failed to create namespace: "+err.Error()); setErr != nil {
			log.Error("Failed to record store error", "error", setErr, "store_id", ks.ID)
		}
		return
	}

	if err := k8s.ApplyKnowledgeSecret(ctx, k8sClient, ks.AccountID, ks.ID, secretName, plainCreds); err != nil {
		log.Error("Failed to create credentials secret", "error", err, "store_id", ks.ID)
		if setErr := ksStore.SetError(ks.ID, "failed to create credentials secret: "+err.Error()); setErr != nil {
			log.Error("Failed to record store error", "error", setErr, "store_id", ks.ID)
		}
		return
	}

	storageClass := ""
	if ks.StorageClass != nil {
		storageClass = *ks.StorageClass
	}
	var publicHost string
	if ks.PublicHost != nil {
		publicHost = *ks.PublicHost
	}
	if err := k8s.ProvisionKnowledgeStore(ctx, k8sClient, k8s.KnowledgeProvisionParams{
		StoreID:        ks.ID,
		AccountID:      ks.AccountID,
		ARN:            ks.ARN,
		Provider:       ks.Provider,
		Storage:        ks.Storage,
		StorageClass:   storageClass,
		SecretName:     secretName,
		Public:         ks.Public,
		PublicHost:     publicHost,
		LocalMode:      cfg.Deployment.K8sClientMode == "local",
		PodSubnetCIDRs: cfg.Deployment.PodSubnetCIDRs,
	}); err != nil {
		log.Error("Failed to provision K8s resources", "error", err, "store_id", ks.ID)
		if setErr := ksStore.SetError(ks.ID, "failed to provision: "+err.Error()); setErr != nil {
			log.Error("Failed to record store error", "error", setErr, "store_id", ks.ID)
		}
		return
	}

	log.Info("Knowledge store K8s resources provisioned", "store_id", ks.ID, "provider", ks.Provider)
}

// ConnectKnowledgeStore onboards an external (bring-your-own) database under an ARN.
// No K8s resources are created — the platform is a credential broker only.
func ConnectKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, cfg *config.Config, queue *riverqueue.Queue, omClient *openmeter.Client, db *sql.DB, entCheck EntitlementChecker) gin.HandlerFunc {
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
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
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

		if err := knowledgestore.ValidateExternalCredentials(req.Provider, creds); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		storeID := deployid.New()
		storeARN := arn.KnowledgeStore(acct.ID, req.Name)

		enc, err := encryptKnowledgeCreds(c.Request.Context(), log, cfg.Deployment.KMSKeyARN, creds)
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
			Mode:             knowledgestore.ModeExternal,
			Status:           knowledgestore.StatusReady,
			EncryptedDataKey: encryptedDataKey,
			KMSKeyARN:        kmsKeyARN,
		})
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				c.JSON(http.StatusConflict, gin.H{"error": "a knowledge store with this name already exists"})
				return
			}
			log.Error("Failed to create external knowledge store record", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create store"})
			return
		}

		if enc != nil {
			if err := ksStore.SaveCredentials(storeID, enc.Credentials); err != nil {
				log.Error("Failed to save credentials", "error", err, "store_id", storeID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save credentials"})
				return
			}
		}

		// If PrivateLink is requested, create the endpoint and enqueue provisioning.
		// The health check is deferred — the DB won't be reachable until the
		// VPC endpoint is accepted and DNS propagates.
		if req.PrivateLink {
			if entCheck != nil {
				if blocked, feature, entResult := entCheck.Check(c.Request.Context(), acct.ID, "knowledge_endpoints"); blocked {
					c.JSON(http.StatusPaymentRequired, middleware.LimitResponse(feature, entResult))
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
				log.Error("Failed to create endpoint record", "error", epErr, "store_id", storeID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create endpoint"})
				return
			}
			if err := ksStore.SetStatus(storeID, knowledgestore.StatusConnecting); err != nil {
				log.Error("Failed to update store status to connecting", "error", err, "store_id", storeID)
			}
			if err := queue.InsertPrivateLinkProvisionJob(c.Request.Context(), storeID); err != nil {
				log.Error("Failed to enqueue PrivateLink provision job", "error", err, "store_id", storeID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue provision job"})
				return
			}

			// Re-read so the response reflects the connecting status.
			ks, _ = ksStore.GetByID(storeID)
			log.Info("External knowledge store connected with PrivateLink", "store_id", storeID, "provider", req.Provider, "arn", storeARN, "region", region)

			go openmeter.EmitActiveKnowledgeStores(context.Background(), omClient, db, log, acct.ID)
			go openmeter.EmitActiveKnowledgeEndpoints(context.Background(), omClient, db, log, acct.ID)

			c.JSON(http.StatusOK, toKnowledgeResponse(ks))
			return
		}

		// Run a connectivity health check unless explicitly skipped.
		if !req.SkipHealthCheck {
			hctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()
			if hErr := knowledgestore.CheckHealth(hctx, req.Provider, creds); hErr != nil {
				msg := fmt.Sprintf("health check failed: %v", hErr)
				log.Warn("External knowledge store health check failed", "store_id", storeID, "provider", req.Provider, "error", hErr)
				if sErr := ksStore.SetError(storeID, msg); sErr != nil {
					log.Error("Failed to set error status after health check failure", "error", sErr, "store_id", storeID)
				}
				// Re-read the store so the response reflects the error status.
				ks, _ = ksStore.GetByID(storeID)
			}
		}

		log.Info("External knowledge store connected", "store_id", storeID, "provider", req.Provider, "arn", storeARN)

		go openmeter.EmitActiveKnowledgeStores(context.Background(), omClient, db, log, acct.ID)

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
			log.Error("Failed to list knowledge stores", "error", err, "account_id", acct.ID)
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

func GetKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil {
			log.Error("Failed to get knowledge store", "error", err)
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

		if k8sClient != nil && (ks.Status == knowledgestore.StatusProvisioning || ks.Status == knowledgestore.StatusConnecting || ks.Status == knowledgestore.StatusPendingAcceptance) {
			ns := k8s.KnowledgeNamespace(acct.ID)
			evts, _ := k8sClient.Clientset().CoreV1().Events(ns).List(c.Request.Context(), metav1.ListOptions{
				FieldSelector: fmt.Sprintf("involvedObject.name=%s-0", k8s.KnowledgeResourceName(ks.ID)),
			})
			if evts != nil {
				for _, e := range evts.Items {
					reason, message := humanizeKnowledgeEvent(e.Reason, e.Message)
					resp.Events = append(resp.Events, KnowledgeEvent{
						Type:    e.Type,
						Reason:  reason,
						Message: message,
						Count:   e.Count,
					})
				}
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
//
// newEC2 and kmsClient are injectable for tests; nil falls back to the real
// AWS-backed clients.
func RecheckKnowledgeStore(
	log *logger.Logger,
	ksStore *knowledgestore.Store,
	newEC2 func(context.Context) (knowledgestore.EC2Client, error),
	kmsClient envelope.KMSClient,
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
			log.Error("Failed to get knowledge store", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get store"})
			return
		}
		if ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}
		if ks.Mode != knowledgestore.ModeExternal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "recheck only applies to connected (external) knowledge stores"})
			return
		}

		ep, err := ksStore.GetEndpoint(ks.ID)
		if err != nil {
			log.Error("Failed to get endpoint", "error", err, "store_id", ks.ID)
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
			log.Error("Failed to record endpoint DNS", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update endpoint"})
			return
		}
		kc, err := recheckKMSClient(c.Request.Context(), kmsClient)
		if err != nil {
			log.Error("Failed to build KMS client", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update host"})
			return
		}
		if err := ksStore.RewriteHostCredential(c.Request.Context(), kc, ks, dns); err != nil {
			log.Error("Failed to rewrite host credential", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update host"})
			return
		}
		if ks.Status != knowledgestore.StatusReady {
			if err := ksStore.SetStatus(ks.ID, knowledgestore.StatusReady); err != nil {
				log.Error("Failed to mark store ready", "error", err, "store_id", ks.ID)
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
		log.Warn("recheck: failed to create EC2 client, using stored DNS", "error", err)
		return ""
	}
	out, err := ec2Client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
		VpcEndpointIds: []string{endpointID},
	})
	if err != nil || out == nil || len(out.VpcEndpoints) == 0 {
		log.Warn("recheck: failed to describe VPC endpoint, using stored DNS", "error", err, "vpce_id", endpointID)
		return ""
	}
	vpce := out.VpcEndpoints[0]
	if strings.ToLower(string(vpce.State)) != "available" || len(vpce.DnsEntries) == 0 {
		return ""
	}
	return aws.ToString(vpce.DnsEntries[0].DnsName)
}

// recheckKMSClient returns the injected client or builds one from the default
// AWS config.
func recheckKMSClient(ctx context.Context, injected envelope.KMSClient) (envelope.KMSClient, error) {
	if injected != nil {
		return injected, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return kms.NewFromConfig(awsCfg), nil
}

func DeleteKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, queue *riverqueue.Queue, omClient *openmeter.Client, db *sql.DB, billing *openmeter.BillingStateManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		ks, err := ksStore.GetByName(acct.ID, c.Param("name"))
		if err != nil {
			log.Error("Failed to get knowledge store", "error", err)
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
			log.Error("Failed to check store bindings", "error", err, "store_id", ks.ID)
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

		if k8sClient != nil && ks.Mode != knowledgestore.ModeExternal {
			if err := k8s.DeleteKnowledgeStore(c.Request.Context(), k8sClient, acct.ID, ks.ID, ks.Public); err != nil {
				log.Error("Failed to delete K8s resources", "error", err, "store_id", ks.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store resources"})
				return
			}
		}

		// Enqueue PrivateLink cleanup if the store has an endpoint.
		// Capture the VPCE ID before the DB row is cascade-deleted.
		if ep, epErr := ksStore.GetEndpoint(ks.ID); epErr == nil && ep != nil && queue != nil {
			endpointID := ""
			if ep.EndpointID != nil {
				endpointID = *ep.EndpointID
			}
			if err := queue.InsertPrivateLinkDeleteJob(c.Request.Context(), ks.ID, endpointID); err != nil {
				log.Error("Failed to enqueue PrivateLink delete job", "error", err, "store_id", ks.ID)
			}
		}

		// No CASCADE: billing row outlives the store for the heartbeat's final-period emission.
		// Block on error — without stopped_at the final period is never emitted.
		if billing != nil && ks.Mode != knowledgestore.ModeExternal {
			if err := billing.StopKnowledgeBilling(c.Request.Context(), ks.ID, time.Now()); err != nil {
				log.Error("Failed to record knowledge billing stop, aborting delete", "error", err, "store_id", ks.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record final usage, please retry"})
				return
			}
		}

		if err := ksStore.Delete(ks.ID); err != nil {
			log.Error("Failed to delete knowledge store record", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store"})
			return
		}

		// Emit updated gauges so entitlement frees up immediately.
		go openmeter.EmitActiveKnowledgeStores(context.Background(), omClient, db, log, acct.ID)
		go openmeter.EmitKnowledgeStorage(context.Background(), omClient, db, log, acct.ID)
		go openmeter.EmitActiveKnowledgeEndpoints(context.Background(), omClient, db, log, acct.ID)

		c.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

func GetKnowledgeStoreLogs(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, lokiClient *loki.Client) gin.HandlerFunc {
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

		ns := k8s.KnowledgeNamespace(acct.ID)
		tailLines := int64(200)
		if tl := c.Query("tailLines"); tl != "" {
			if parsed, err := strconv.ParseInt(tl, 10, 64); err == nil && parsed > 0 {
				tailLines = parsed
			}
		}

		// Knowledge stores are long-lived and often quiet after startup,
		// so default to a 24h lookback (vs 1h for agent deployments).
		lokiParams := loki.QueryParams{
			Namespace: ns,
			Workload:  k8s.KnowledgeResourceName(ks.ID),
			Container: "app",
			Limit:     tailLines,
			Start:     time.Now().Add(-24 * time.Hour),
			Direction: "backward",
		}
		if s := c.Query("since"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				lokiParams.Start = t
			} else if ns, err := strconv.ParseInt(s, 10, 64); err == nil {
				lokiParams.Start = time.Unix(0, ns)
			}
		}
		if u := c.Query("until"); u != "" {
			if t, err := time.Parse(time.RFC3339, u); err == nil {
				lokiParams.End = t
			} else if ns, err := strconv.ParseInt(u, 10, 64); err == nil {
				lokiParams.End = time.Unix(0, ns)
			}
		}

		loc := getTimezoneLocation(c)

		streamLogs(c, log,
			lokiClient, lokiParams,
			k8sClient, ns, k8s.KnowledgeResourceName(ks.ID)+"-0", &corev1.PodLogOptions{Container: "app", TailLines: &tailLines, Timestamps: true},
			loc,
		)
	}
}

// StreamKnowledgeStoreLogs streams knowledge store logs via SSE, matching the
// deployment log stream contract (ready/status/heartbeat events + JSON log data).
func StreamKnowledgeStoreLogs(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, lokiClient *loki.Client) gin.HandlerFunc {
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

		if lokiClient == nil && k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "log backend not configured"})
			return
		}

		ns := k8s.KnowledgeNamespace(acct.ID)
		resourceName := k8s.KnowledgeResourceName(ks.ID)
		podName := resourceName + "-0"

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
		flusher := c.Writer.(http.Flusher)

		fmt.Fprintf(c.Writer, "event: ready\ndata: {}\n\n") //nolint:errcheck
		flusher.Flush()

		loc := getTimezoneLocation(c)

		writeEvent := func(ll loki.LogLine) bool {
			payload, _ := json.Marshal(lokiLineToEntry(ll, loc))
			_, writeErr := fmt.Fprintf(c.Writer, "id: %d\ndata: %s\n\n", ll.Timestamp.UnixNano(), payload)
			if writeErr != nil {
				return false
			}
			flusher.Flush()
			return true
		}

		writeStatusEvent := func(status string) bool {
			_, err := fmt.Fprintf(c.Writer, "event: status\ndata: {\"status\":%q}\n\n", status)
			if err != nil {
				return false
			}
			flusher.Flush()
			return true
		}

		writeErrorEvent := func(message string) {
			fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":%q}\n\n", message) //nolint:errcheck
			flusher.Flush()
		}

		writeHeartbeat := func() bool {
			_, err := fmt.Fprintf(c.Writer, "event: heartbeat\ndata: {}\n\n")
			if err != nil {
				return false
			}
			flusher.Flush()
			return true
		}

		reconnectPause := func() bool {
			select {
			case <-time.After(500 * time.Millisecond):
				return true
			case <-c.Request.Context().Done():
				return false
			}
		}

		if lokiClient != nil {
			heartbeat := time.NewTicker(5 * time.Second)
			defer heartbeat.Stop()
			firstConnect := true

			for {
				if !writeStatusEvent("connecting") {
					return
				}
				ch, tailErr := lokiClient.TailLogs(c.Request.Context(), loki.QueryParams{
					Namespace: ns,
					Workload:  resourceName,
					Container: "app",
					Start:     time.Now(),
				})
				if tailErr != nil {
					if firstConnect {
						writeErrorEvent("failed to connect to log stream")
						return
					}
					if !reconnectPause() {
						return
					}
					continue
				}
				firstConnect = false
				if !writeStatusEvent("streaming") {
					return
				}

			inner:
				for {
					select {
					case ll, ok := <-ch:
						if !ok {
							if !writeStatusEvent("reconnecting") {
								return
							}
							break inner
						}
						if !writeEvent(ll) {
							return
						}
					case <-heartbeat.C:
						if !writeHeartbeat() {
							return
						}
					case <-c.Request.Context().Done():
						return
					}
				}
				if !reconnectPause() {
					return
				}
			}
		}

		// K8s fallback: stream from the known pod with Follow=true.
		sinceTime := metav1.NewTime(time.Now())
		logOpts := &corev1.PodLogOptions{
			Container:  "app",
			Follow:     true,
			SinceTime:  &sinceTime,
			Timestamps: true,
		}

		if !writeStatusEvent("connecting") {
			return
		}
		stream, streamErr := k8sClient.Clientset().CoreV1().Pods(ns).GetLogs(podName, logOpts).Stream(c.Request.Context())
		if streamErr != nil {
			writeErrorEvent("failed to stream pod logs")
			return
		}
		defer stream.Close() //nolint:errcheck

		if !writeStatusEvent("streaming") {
			return
		}

		lines := make(chan loki.LogLine)
		go func() {
			defer close(lines)
			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				line := scanner.Text()
				ts := time.Now()
				msg := line
				if idx := strings.IndexByte(line, ' '); idx > 0 {
					if t, parseErr := time.Parse(time.RFC3339Nano, line[:idx]); parseErr == nil {
						ts = t
						msg = line[idx+1:]
					}
				}
				select {
				case lines <- loki.LogLine{Timestamp: ts, Line: msg}:
				case <-c.Request.Context().Done():
					return
				}
			}
		}()

		heartbeat := time.NewTicker(5 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case ll, ok := <-lines:
				if !ok {
					return
				}
				if !writeEvent(ll) {
					return
				}
			case <-heartbeat.C:
				if !writeHeartbeat() {
					return
				}
			case <-c.Request.Context().Done():
				return
			}
		}
	}
}

// GetKnowledgeStoreMetrics returns infrastructure metrics for a knowledge store.
func GetKnowledgeStoreMetrics(log *logger.Logger, ksStore *knowledgestore.Store, promClient *promquery.Client) gin.HandlerFunc {
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

		resp := KnowledgeMetricsResponse{
			UptimeSeconds: int64(time.Since(ks.CreatedAt).Seconds()),
		}

		// Parse allocated storage into bytes for the response.
		if ks.Storage != "" {
			resp.StorageTotal = parseStorageBytes(ks.Storage)
		}

		if promClient == nil {
			c.JSON(http.StatusOK, resp)
			return
		}

		ns := k8s.KnowledgeNamespace(acct.ID)
		pod := k8s.KnowledgeResourceName(ks.ID) + "-0"
		cluster := promClient.Cluster()

		clusterFilter := ""
		if cluster != "" {
			clusterFilter = fmt.Sprintf(`,cluster="%s"`, cluster)
		}

		// CPU usage (cores) — 5m rate
		cpuQL := fmt.Sprintf(
			`sum(rate(container_cpu_usage_seconds_total{namespace="%s",pod="%s",container="app"%s}[5m]))`,
			ns, pod, clusterFilter,
		)
		if samples, err := promClient.Query(c.Request.Context(), cpuQL); err == nil && len(samples) > 0 {
			v := samples[0].Value
			resp.CPUCores = &v
		}

		// Memory working set (bytes)
		memQL := fmt.Sprintf(
			`sum(container_memory_working_set_bytes{namespace="%s",pod="%s",container="app"%s})`,
			ns, pod, clusterFilter,
		)
		if samples, err := promClient.Query(c.Request.Context(), memQL); err == nil && len(samples) > 0 {
			v := int64(samples[0].Value)
			resp.MemoryBytes = &v
		}

		// PVC storage used (bytes)
		resourceName := k8s.KnowledgeResourceName(ks.ID)
		storageQL := fmt.Sprintf(
			`kubelet_volume_stats_used_bytes{namespace="%s",persistentvolumeclaim=~".*%s.*"%s}`,
			ns, resourceName, clusterFilter,
		)
		if samples, err := promClient.Query(c.Request.Context(), storageQL); err == nil && len(samples) > 0 {
			v := int64(samples[0].Value)
			resp.StorageUsed = &v
		}

		c.JSON(http.StatusOK, resp)
	}
}

// parseStorageBytes converts K8s storage strings like "10Gi" to bytes.
func parseStorageBytes(s string) *int64 {
	s = strings.TrimSpace(s)
	var multiplier int64
	if strings.HasSuffix(s, "Gi") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "Gi")
	} else if strings.HasSuffix(s, "Mi") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "Mi")
	} else if strings.HasSuffix(s, "Ti") {
		multiplier = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "Ti")
	} else {
		return nil
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	result := val * multiplier
	return &result
}

func GetKnowledgeStoreEvents(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient) gin.HandlerFunc {
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

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "k8s not configured"})
			return
		}

		ns := k8s.KnowledgeNamespace(acct.ID)
		ctx := c.Request.Context()

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		// Track emitted event UIDs to avoid re-sending on each tick.
		seen := make(map[string]struct{})

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, err := ksStore.GetByID(ks.ID)
				if err != nil || current == nil {
					_, _ = fmt.Fprintf(c.Writer, "data: {\"error\":\"store not found\"}\n\n")
					c.Writer.Flush()
					return
				}

				errMsg := ""
				if current.Error != nil {
					errMsg = *current.Error
				}
				_, _ = fmt.Fprintf(c.Writer, "data: {\"status\":%q,\"store_id\":%q,\"error\":%q}\n\n", current.Status, current.ID, errMsg)

				events, _ := k8sClient.Clientset().CoreV1().Events(ns).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.name=%s-0", k8s.KnowledgeResourceName(ks.ID)),
				})
				if events != nil {
					for _, evt := range events.Items {
						uid := string(evt.UID)
						if _, already := seen[uid]; already {
							continue
						}
						seen[uid] = struct{}{}
						reason, message := humanizeKnowledgeEvent(evt.Reason, evt.Message)
						_, _ = fmt.Fprintf(c.Writer, "data: {\"type\":%q,\"reason\":%q,\"message\":%q}\n\n",
							evt.Type, reason, message)
					}
				}
				c.Writer.Flush()

				if current.Status == knowledgestore.StatusReady || current.Status == knowledgestore.StatusError {
					return
				}
			}
		}
	}
}

// GetKnowledgeStoreCredentials resolves and returns the store's credentials.
// Uses KMS decryption when available, falls back to reading the k8s Secret.
func GetKnowledgeStoreCredentials(log *logger.Logger, ksStore *knowledgestore.Store, secretReader knowledgestore.SecretReader) gin.HandlerFunc {
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
			log.Error("Failed to get credentials", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve credentials"})
			return
		}

		var kmsClient envelope.KMSClient
		if len(ks.EncryptedDataKey) > 0 {
			awsCfg, awsErr := awsconfig.LoadDefaultConfig(c.Request.Context())
			if awsErr != nil {
				log.Error("Failed to load AWS config", "error", awsErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize KMS"})
				return
			}
			kmsClient = kms.NewFromConfig(awsCfg)
		}

		storeNS := k8s.KnowledgeNamespace(ks.AccountID)
		plainCreds, resolveErr := knowledgestore.ResolveCredentials(
			c.Request.Context(), ks, creds, kmsClient, secretReader, storeNS,
		)
		if resolveErr != nil {
			log.Error("Failed to resolve credentials", "error", resolveErr, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve credentials"})
			return
		}

		c.JSON(http.StatusOK, plainCreds)
	}
}
