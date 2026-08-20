package riverqueue

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// KnowledgeReconcileArgs are the job arguments for the knowledge store reconciler.
type KnowledgeReconcileArgs struct{}

func (KnowledgeReconcileArgs) Kind() string { return "knowledge.reconcile" }

func (KnowledgeReconcileArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[KnowledgeReconcileArgs]()
}

// KnowledgeReconcileWorker reconciles knowledge store state. It runs
// periodically and polls PrivateLink endpoints (connecting/pending-acceptance),
// advancing them as their VPC endpoint status changes in AWS.
type KnowledgeReconcileWorker struct {
	river.WorkerDefaults[KnowledgeReconcileArgs]
	ksStore *knowledgestore.Store
	log     *logger.Logger

	vault *envelope.Vault
}

func (w *KnowledgeReconcileWorker) Work(ctx context.Context, _ *river.Job[KnowledgeReconcileArgs]) error {
	w.reconcilePrivateLink(ctx)

	return nil
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

		// AWS returns VPC endpoint states in lowercase ("available") but the
		// SDK enum constants are PascalCase ("Available"). Normalise to
		// lowercase before comparing.
		vpce := out.VpcEndpoints[0]
		state := strings.ToLower(string(vpce.State))
		switch state {
		case "pendingacceptance":
			if ep.Status != knowledgestore.StatusPendingAcceptance {
				_ = w.ksStore.SetEndpointStatus(ep.KnowledgeStoreID, knowledgestore.StatusPendingAcceptance)
				_ = w.ksStore.SetStatus(ep.KnowledgeStoreID, knowledgestore.StatusPendingAcceptance)
			}

		case "available":
			// DNS entries may take a few seconds to propagate after the VPCE
			// transitions to available. Defer to next reconcile cycle if empty.
			if len(vpce.DnsEntries) == 0 || aws.ToString(vpce.DnsEntries[0].DnsName) == "" {
				w.log.Info("KnowledgeReconcile: VPCE available but DNS not yet propagated, will retry",
					"store_id", ep.KnowledgeStoreID, "vpce_id", *ep.EndpointID)
				continue
			}
			dns := aws.ToString(vpce.DnsEntries[0].DnsName)

			if err := w.ksStore.SetEndpointReady(ep.KnowledgeStoreID, *ep.EndpointID, dns); err != nil {
				w.log.Error("KnowledgeReconcile: failed to mark endpoint ready",
					"error", err, "store_id", ep.KnowledgeStoreID)
				continue
			}
			// The user-supplied host was the "com.amazonaws.vpce.*" service name,
			// which isn't connectable. Now that the VPC endpoint has resolved,
			// rewrite the store's HOST credential to the real endpoint DNS so the
			// stored value is the address agents actually dial. Done before the
			// store flips to ready so a deploy never observes the stale host.
			if err := w.persistResolvedHost(ctx, ep.KnowledgeStoreID, dns); err != nil {
				w.log.Error("KnowledgeReconcile: failed to persist resolved host",
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

		case "rejected", "failed", "deleted":
			reason := fmt.Sprintf("VPC endpoint %s: %s", *ep.EndpointID, vpce.State)
			w.setEndpointAndStoreError(ep.KnowledgeStoreID, reason)

		default:
			w.log.Warn("KnowledgeReconcile: unhandled VPCE state",
				"store_id", ep.KnowledgeStoreID, "vpce_id", *ep.EndpointID, "state", string(vpce.State))
		}
	}
}

// persistResolvedHost rewrites a store's HOST credential to the resolved
// PrivateLink endpoint DNS. The new value is encrypted under the store's
// existing data key so it decrypts alongside the store's other credentials;
// SaveCredentials upserts only the HOST row, leaving the rest untouched.
//
// External-store credentials require KMS (they have no k8s Secret fallback), so
// a store without an encrypted data key has no persisted credentials to update
// and is skipped.
func (w *KnowledgeReconcileWorker) persistResolvedHost(ctx context.Context, storeID, dns string) error {
	store, err := w.ksStore.GetByID(storeID)
	if err != nil {
		return fmt.Errorf("get store: %w", err)
	}
	if store == nil || len(store.EncryptedDataKey) == 0 {
		return nil
	}

	return w.ksStore.RewriteHostCredential(ctx, w.vault, store, dns)
}

func (w *KnowledgeReconcileWorker) setEndpointAndStoreError(storeID, errMsg string) {
	if err := w.ksStore.SetEndpointError(storeID, errMsg); err != nil {
		w.log.Error("KnowledgeReconcile: failed to record endpoint error", "error", err, "store_id", storeID)
	}
	if err := w.ksStore.SetError(storeID, errMsg); err != nil {
		w.log.Error("KnowledgeReconcile: failed to record store error", "error", err, "store_id", storeID)
	}
}
