package riverqueue

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// KnowledgeReconcileArgs are the job arguments for the knowledge store reconciler.
type KnowledgeReconcileArgs struct{}

func (KnowledgeReconcileArgs) Kind() string { return "knowledge_reconcile" }

// KnowledgeReconcileWorker reconciles knowledge store state.
//
// It runs periodically and does three things:
//  1. Advances provisioning managed stores to ready/error once their StatefulSet is healthy.
//  2. Recreates missing K8s credentials secrets for ready managed stores (cluster migration recovery).
//  3. Polls PrivateLink endpoints (connecting/pending-acceptance) and advances them.
type KnowledgeReconcileWorker struct {
	river.WorkerDefaults[KnowledgeReconcileArgs]
	ksStore *knowledgestore.Store
	k8s     k8s.ClusterClient
	log     *logger.Logger
}

func (w *KnowledgeReconcileWorker) Work(ctx context.Context, _ *river.Job[KnowledgeReconcileArgs]) error {
	if w.k8s != nil {
		w.reconcileProvisioning(ctx)
		w.ensureSecrets(ctx)
	}
	w.reconcilePrivateLink(ctx)

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
			// LB is assigned — the friendly CNAME (name.account.knowledge.domain)
			// was already recorded by the create handler. External-dns handles
			// the CNAME → NLB mapping; we only needed the LB check as a
			// readiness gate.
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

// reconcilePrivateLink checks PrivateLink endpoints in connecting/pending-acceptance
// states and advances them based on the VPC endpoint status from AWS.
func (w *KnowledgeReconcileWorker) reconcilePrivateLink(ctx context.Context) {
	endpoints, err := w.ksStore.ListEndpointsByStatus(
		knowledgestore.StatusConnecting,
		knowledgestore.StatusPendingAcceptance,
	)
	if err != nil {
		w.log.Error("KnowledgeReconcile: failed to list PrivateLink endpoints", "error", err)
		return
	}
	if len(endpoints) == 0 {
		return
	}

	// Lazy-load EC2 client only when there are endpoints to check.
	var ec2Client knowledgestore.EC2Client
	for _, ep := range endpoints {
		if ep.EndpointID == nil || *ep.EndpointID == "" {
			// VPCE not yet created — provision worker hasn't run yet.
			continue
		}

		if ec2Client == nil {
			ec2Client, err = knowledgestore.NewEC2Client(ctx)
			if err != nil {
				w.log.Error("KnowledgeReconcile: failed to create EC2 client", "error", err)
				return
			}
		}

		out, err := ec2Client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
			VpcEndpointIds: []string{*ep.EndpointID},
		})
		if err != nil {
			w.log.Error("KnowledgeReconcile: failed to describe VPC endpoint",
				"error", err, "store_id", ep.KnowledgeStoreID, "vpce_id", *ep.EndpointID)
			continue
		}
		if len(out.VpcEndpoints) == 0 {
			w.setEndpointAndStoreError(ep.KnowledgeStoreID, "VPC endpoint not found")
			continue
		}

		vpce := out.VpcEndpoints[0]
		switch vpce.State {
		case ec2types.StatePendingAcceptance:
			if ep.Status != knowledgestore.StatusPendingAcceptance {
				_ = w.ksStore.SetEndpointStatus(ep.KnowledgeStoreID, knowledgestore.StatusPendingAcceptance)
				_ = w.ksStore.SetStatus(ep.KnowledgeStoreID, knowledgestore.StatusPendingAcceptance)
			}

		case ec2types.StateAvailable:
			dns := ""
			if len(vpce.DnsEntries) > 0 {
				dns = aws.ToString(vpce.DnsEntries[0].DnsName)
			}
			if err := w.ksStore.SetEndpointReady(ep.KnowledgeStoreID, *ep.EndpointID, dns); err != nil {
				w.log.Error("KnowledgeReconcile: failed to mark endpoint ready",
					"error", err, "store_id", ep.KnowledgeStoreID)
				continue
			}
			if err := w.ksStore.SetStatus(ep.KnowledgeStoreID, knowledgestore.StatusReady); err != nil {
				w.log.Error("KnowledgeReconcile: failed to mark store ready",
					"error", err, "store_id", ep.KnowledgeStoreID)
				continue
			}
			w.log.Info("KnowledgeReconcile: PrivateLink endpoint ready",
				"store_id", ep.KnowledgeStoreID, "vpce_id", *ep.EndpointID, "dns", dns)

		case ec2types.StateRejected, ec2types.StateFailed, ec2types.StateDeleted:
			reason := fmt.Sprintf("VPC endpoint %s: %s", *ep.EndpointID, vpce.State)
			w.setEndpointAndStoreError(ep.KnowledgeStoreID, reason)
		}
	}
}

func (w *KnowledgeReconcileWorker) setEndpointAndStoreError(storeID, errMsg string) {
	if err := w.ksStore.SetEndpointError(storeID, errMsg); err != nil {
		w.log.Error("KnowledgeReconcile: failed to record endpoint error", "error", err, "store_id", storeID)
	}
	if err := w.ksStore.SetError(storeID, errMsg); err != nil {
		w.log.Error("KnowledgeReconcile: failed to record store error", "error", err, "store_id", storeID)
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
