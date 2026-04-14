package riverqueue

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
)

// KnowledgeReconcileArgs are the job arguments for the knowledge store reconciler.
type KnowledgeReconcileArgs struct{}

func (KnowledgeReconcileArgs) Kind() string { return "knowledge_reconcile" }

// KnowledgeReconcileWorker reconciles managed knowledge store state.
//
// It runs periodically and does two things:
//  1. Advances provisioning stores to ready/error once their StatefulSet is healthy.
//  2. Recreates missing K8s credentials secrets for ready stores (cluster migration recovery).
type KnowledgeReconcileWorker struct {
	river.WorkerDefaults[KnowledgeReconcileArgs]
	ksStore *knowledgestore.Store
	k8s     k8s.ClusterClient
	log     *logger.Logger
}

func (w *KnowledgeReconcileWorker) Work(ctx context.Context, _ *river.Job[KnowledgeReconcileArgs]) error {
	if w.k8s == nil {
		return nil
	}

	w.reconcileProvisioning(ctx)
	w.ensureSecrets(ctx)

	return nil
}

// reconcileProvisioning checks stores in provisioning state and advances them.
func (w *KnowledgeReconcileWorker) reconcileProvisioning(ctx context.Context) {
	stores, err := w.ksStore.ListProvisioning()
	if err != nil {
		w.log.Error("KnowledgeReconcile: failed to list provisioning stores", "error", err)
		return
	}

	for _, ks := range stores {
		ready, err := k8s.IsStatefulSetReady(ctx, w.k8s, ks.AccountID, ks.ID)
		if err != nil {
			w.log.Error("KnowledgeReconcile: failed to check StatefulSet readiness",
				"error", err, "store_id", ks.ID)
			continue
		}

		if !ready {
			continue
		}

		// For public stores: wait for LB hostname before marking ready.
		if ks.Public {
			host, err := k8s.GetLoadBalancerHostname(ctx, w.k8s, ks.AccountID, ks.ID)
			if err != nil {
				w.log.Error("KnowledgeReconcile: failed to get LB hostname",
					"error", err, "store_id", ks.ID)
				continue
			}
			if host == "" {
				// LB not yet assigned — check again next cycle.
				continue
			}
			// DNS CNAME creation is handled by external DNS controller (e.g. external-dns).
			// Only record the LB hostname if the create handler didn't already
			// set a friendly CNAME (e.g. name.account.knowledge.domain).
			if ks.PublicHost == nil || *ks.PublicHost == "" {
				if err := w.ksStore.SetPublicHost(ks.ID, host); err != nil {
					w.log.Error("KnowledgeReconcile: failed to set public host",
						"error", err, "store_id", ks.ID)
					continue
				}
			}
		}

		if err := w.ksStore.SetStatus(ks.ID, knowledgestore.StatusReady); err != nil {
			w.log.Error("KnowledgeReconcile: failed to mark store ready",
				"error", err, "store_id", ks.ID)
			continue
		}

		w.log.Info("KnowledgeReconcile: store ready", "store_id", ks.ID, "provider", ks.Provider)
	}
}

// ensureSecrets checks ready stores and recreates missing K8s credentials secrets.
// This is the recovery path for cluster migrations and accidental secret deletions.
// Decryption requires the KMS data key stored in the DB — if KMS is unavailable or
// the store has no encrypted credentials, the secret cannot be recreated.
func (w *KnowledgeReconcileWorker) ensureSecrets(ctx context.Context) {
	stores, err := w.ksStore.ListReady()
	if err != nil {
		w.log.Error("KnowledgeReconcile: failed to list ready stores", "error", err)
		return
	}

	// Load KMS client once — only if any store actually needs secret recovery.
	var kmsClient *awskms.Client
	for _, ks := range stores {
		secretName := ks.ID + "-credentials"
		exists, err := k8s.SecretExists(ctx, w.k8s, ks.AccountID, secretName)
		if err != nil {
			w.log.Error("KnowledgeReconcile: failed to check secret existence",
				"error", err, "store_id", ks.ID)
			continue
		}
		if exists {
			continue
		}

		if len(ks.EncryptedDataKey) == 0 || ks.KMSKeyARN == nil {
			w.log.Warn("KnowledgeReconcile: missing secret but no encrypted credentials to recover from",
				"store_id", ks.ID)
			continue
		}

		creds, err := w.ksStore.GetCredentials(ks.ID)
		if err != nil || len(creds) == 0 {
			w.log.Warn("KnowledgeReconcile: missing secret and no credentials in DB",
				"store_id", ks.ID, "error", err)
			continue
		}

		if kmsClient == nil {
			kmsClient, err = loadKMSClient(ctx)
			if err != nil {
				w.log.Error("KnowledgeReconcile: failed to load KMS client", "error", err)
				return
			}
		}

		plainCreds, decErr := knowledgestore.DecryptCredentials(ctx, kmsClient, ks.EncryptedDataKey, creds)
		if decErr != nil {
			w.log.Error("KnowledgeReconcile: failed to decrypt credentials",
				"error", decErr, "store_id", ks.ID)
			continue
		}

		if err := k8s.ApplyKnowledgeSecret(ctx, w.k8s, ks.AccountID, ks.ID, secretName, plainCreds); err != nil {
			w.log.Error("KnowledgeReconcile: failed to recreate secret",
				"error", err, "store_id", ks.ID)
			continue
		}

		w.log.Info("KnowledgeReconcile: recreated missing credentials secret", "store_id", ks.ID)
	}
}

// loadKMSClient loads the default AWS config and returns a KMS client.
func loadKMSClient(ctx context.Context) (*awskms.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return awskms.NewFromConfig(cfg), nil
}
