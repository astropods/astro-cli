package riverqueue

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// PrivateLinkDeleteArgs are the job arguments for deleting a VPC endpoint.
// EndpointID is captured before the DB row is deleted (ON DELETE CASCADE).
type PrivateLinkDeleteArgs struct {
	StoreID    string `json:"store_id"`
	EndpointID string `json:"endpoint_id"`
}

func (PrivateLinkDeleteArgs) Kind() string { return "privatelink_delete" }

// PrivateLinkDeleteWorker deletes AWS VPC endpoints for external knowledge stores.
type PrivateLinkDeleteWorker struct {
	river.WorkerDefaults[PrivateLinkDeleteArgs]
	ksStore *knowledgestore.Store
	log     *logger.Logger
}

func (w *PrivateLinkDeleteWorker) Work(ctx context.Context, job *river.Job[PrivateLinkDeleteArgs]) error {
	storeID := job.Args.StoreID
	endpointID := job.Args.EndpointID

	if endpointID != "" {
		ec2Client, err := knowledgestore.NewEC2Client(ctx)
		if err != nil {
			return fmt.Errorf("create ec2 client: %w", err)
		}

		_, err = ec2Client.DeleteVpcEndpoints(ctx, &ec2.DeleteVpcEndpointsInput{
			VpcEndpointIds: []string{endpointID},
		})
		if err != nil {
			w.log.Error("PrivateLinkDelete: failed to delete VPC endpoint",
				"error", err, "store_id", storeID, "endpoint_id", endpointID)
			return fmt.Errorf("delete vpc endpoint: %w", err)
		}

		w.log.Info("PrivateLinkDelete: VPC endpoint deleted",
			"store_id", storeID, "endpoint_id", endpointID)
	}

	// Clean up the DB row — may already be gone via ON DELETE CASCADE.
	if err := w.ksStore.DeleteEndpoint(storeID); err != nil {
		w.log.Warn("PrivateLinkDelete: failed to delete endpoint row (may already be cascaded)",
			"error", err, "store_id", storeID)
	}

	return nil
}
