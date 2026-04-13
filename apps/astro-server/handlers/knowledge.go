package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	spec "github.com/astropods/astro/packages/astro-spec"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type knowledgeResponse struct {
	ID         string    `json:"id"`
	ARN        string    `json:"arn"`
	Name       string    `json:"name"`
	Provider   string    `json:"provider"`
	Status     string    `json:"status"`
	Storage    string    `json:"storage"`
	Public     bool      `json:"public"`
	PublicHost *string   `json:"public_host,omitempty"`
	Error      *string   `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func toKnowledgeResponse(ks *knowledgestore.KnowledgeStore) knowledgeResponse {
	return knowledgeResponse{
		ID:         ks.ID,
		ARN:        ks.ARN,
		Name:       ks.Name,
		Provider:   ks.Provider,
		Status:     ks.Status,
		Storage:    ks.Storage,
		Public:     ks.Public,
		PublicHost: ks.PublicHost,
		Error:      ks.Error,
		CreatedAt:  ks.CreatedAt,
		UpdatedAt:  ks.UpdatedAt,
	}
}

func CreateKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "account not found in context"})
			return
		}

		var req struct {
			Name     string `json:"name" binding:"required"`
			Provider string `json:"provider" binding:"required"`
			Storage  string `json:"storage"`
			Public   bool   `json:"public"`
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

		storeID := deployid.New()
		arn := fmt.Sprintf("arn:knowledge:%s:%s", acct.ID, req.Name)
		ns := k8s.KnowledgeNamespace(acct.ID)

		var publicHost string
		if req.Public && cfg.Deployment.KnowledgeDomain != "" {
			publicHost = fmt.Sprintf("%s.%s.%s", req.Name, acct.Name, cfg.Deployment.KnowledgeDomain)
		}

		plainCreds, err := knowledgestore.GenerateCredentials(req.Provider)
		if err != nil {
			log.Error("Failed to generate credentials", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate credentials"})
			return
		}

		var encryptedDataKey []byte
		var kmsKeyARN string
		var encryptedCreds []knowledgestore.Credential

		if cfg.Deployment.KMSKeyARN != "" {
			awsCfg, err := awsconfig.LoadDefaultConfig(c.Request.Context())
			if err != nil {
				log.Error("Failed to load AWS config", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize KMS"})
				return
			}
			kmsClient := kms.NewFromConfig(awsCfg)
			enc, err := envelope.NewEncryptor(c.Request.Context(), kmsClient, cfg.Deployment.KMSKeyARN)
			if err != nil {
				log.Error("Failed to create KMS encryptor", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt credentials"})
				return
			}
			encryptedCreds, err = knowledgestore.EncryptCredentials(enc, plainCreds)
			if err != nil {
				log.Error("Failed to encrypt credentials", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt credentials"})
				return
			}
			encryptedDataKey = enc.EncryptedDataKey
			kmsKeyARN = cfg.Deployment.KMSKeyARN
		}

		ks, err := ksStore.Create(knowledgestore.CreateParams{
			ID:               storeID,
			AccountID:        acct.ID,
			Name:             req.Name,
			ARN:              arn,
			Provider:         req.Provider,
			Namespace:        ns,
			Storage:          storage,
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

		if len(encryptedCreds) > 0 {
			if err := ksStore.SaveCredentials(storeID, encryptedCreds); err != nil {
				// Non-fatal — reconciler will not be able to recover the secret, but the
				// K8s secret created in provisionStoreAsync will still work until cluster migration.
				log.Error("Failed to save credentials", "error", err, "store_id", storeID)
			}
		}

		if k8sClient != nil {
			go provisionStoreAsync(context.Background(), log, ksStore, k8sClient, ks, plainCreds, cfg)
		}

		c.JSON(http.StatusAccepted, toKnowledgeResponse(ks))
	}
}

// provisionStoreAsync creates K8s resources after the 202 is sent. Uses context.Background()
// intentionally — the request context is already cancelled by the time this runs.
func provisionStoreAsync(ctx context.Context, log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient, ks *knowledgestore.KnowledgeStore, plainCreds map[string]string, cfg *config.Config) {
	secretName := ks.ID + "-credentials"

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

	if err := k8s.ProvisionKnowledgeStore(ctx, k8sClient, k8s.KnowledgeProvisionParams{
		StoreID:    ks.ID,
		AccountID:  ks.AccountID,
		ARN:        ks.ARN,
		Provider:   ks.Provider,
		Storage:    ks.Storage,
		SecretName: secretName,
		Public:     ks.Public,
		LocalMode:  cfg.Deployment.K8sClientMode == "local",
	}); err != nil {
		log.Error("Failed to provision K8s resources", "error", err, "store_id", ks.ID)
		if setErr := ksStore.SetError(ks.ID, "failed to provision: "+err.Error()); setErr != nil {
			log.Error("Failed to record store error", "error", setErr, "store_id", ks.ID)
		}
		return
	}

	log.Info("Knowledge store K8s resources provisioned", "store_id", ks.ID, "provider", ks.Provider)
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

		resp := make([]knowledgeResponse, 0, len(stores))
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
			log.Error("Failed to get knowledge store", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get store"})
			return
		}
		if ks == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
			return
		}

		c.JSON(http.StatusOK, toKnowledgeResponse(ks))
	}
}

func DeleteKnowledgeStore(log *logger.Logger, ksStore *knowledgestore.Store, k8sClient k8s.ClusterClient) gin.HandlerFunc {
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

		if k8sClient != nil {
			if err := k8s.DeleteKnowledgeStore(c.Request.Context(), k8sClient, acct.ID, ks.ID, ks.Public); err != nil {
				log.Error("Failed to delete K8s resources", "error", err, "store_id", ks.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store resources"})
				return
			}
		}

		if err := ksStore.Delete(ks.ID); err != nil {
			log.Error("Failed to delete knowledge store record", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store"})
			return
		}

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

		streamLogs(c, log,
			lokiClient, loki.QueryParams{Namespace: ns, Workload: ks.ID, Limit: tailLines},
			k8sClient, ns, ks.ID+"-0", &corev1.PodLogOptions{Container: "app", TailLines: &tailLines},
		)
	}
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
					FieldSelector: fmt.Sprintf("involvedObject.name=%s-0", ks.ID),
				})
				if events != nil {
					for _, evt := range events.Items {
						uid := string(evt.UID)
						if _, already := seen[uid]; already {
							continue
						}
						seen[uid] = struct{}{}
						_, _ = fmt.Fprintf(c.Writer, "data: {\"type\":%q,\"reason\":%q,\"message\":%q}\n\n",
							evt.Type, evt.Reason, evt.Message)
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

// GetKnowledgeStoreCredentials decrypts and returns the store's credentials.
// Reads from the DB — does not depend on the K8s secret existing.
func GetKnowledgeStoreCredentials(log *logger.Logger, ksStore *knowledgestore.Store) gin.HandlerFunc {
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

		if len(creds) == 0 {
			// KMS was not configured at creation time.
			c.JSON(http.StatusNotFound, gin.H{"error": "credentials not stored (KMS was not configured at creation)"})
			return
		}

		if len(ks.EncryptedDataKey) == 0 || ks.KMSKeyARN == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "missing encryption key"})
			return
		}

		awsCfg, err := awsconfig.LoadDefaultConfig(c.Request.Context())
		if err != nil {
			log.Error("Failed to load AWS config", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize KMS"})
			return
		}
		kmsClient := kms.NewFromConfig(awsCfg)

		plainCreds, err := knowledgestore.DecryptCredentials(c.Request.Context(), kmsClient, ks.EncryptedDataKey, creds)
		if err != nil {
			log.Error("Failed to decrypt credentials", "error", err, "store_id", ks.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decrypt credentials"})
			return
		}

		c.JSON(http.StatusOK, plainCreds)
	}
}
