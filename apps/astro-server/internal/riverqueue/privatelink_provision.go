package riverqueue

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// PrivateLinkProvisionArgs are the job arguments for creating a VPC endpoint.
type PrivateLinkProvisionArgs struct {
	StoreID string `json:"store_id"`
}

func (PrivateLinkProvisionArgs) Kind() string { return "privatelink_provision" }

// PrivateLinkProvisionWorker creates AWS VPC endpoints for external knowledge stores.
type PrivateLinkProvisionWorker struct {
	river.WorkerDefaults[PrivateLinkProvisionArgs]
	ksStore *knowledgestore.Store
	cfg     *config.Config
	log     *logger.Logger
}

func (w *PrivateLinkProvisionWorker) Work(ctx context.Context, job *river.Job[PrivateLinkProvisionArgs]) error {
	storeID := job.Args.StoreID

	ep, err := w.ksStore.GetEndpoint(storeID)
	if err != nil {
		return fmt.Errorf("get endpoint: %w", err)
	}
	if ep == nil {
		w.log.Warn("PrivateLinkProvision: endpoint not found, skipping", "store_id", storeID)
		return nil
	}

	ec2Client, err := knowledgestore.NewEC2Client(ctx)
	if err != nil {
		w.setError(storeID, "failed to create EC2 client: "+err.Error())
		return fmt.Errorf("create ec2 client: %w", err)
	}

	// Pre-flight: check VPC endpoint count (AWS default limit is 50 per VPC).
	if err := w.checkVPCEndpointLimit(ctx, ec2Client); err != nil {
		w.setError(storeID, err.Error())
		return err
	}

	out, err := ec2Client.CreateVpcEndpoint(ctx, &ec2.CreateVpcEndpointInput{
		VpcEndpointType:  ec2types.VpcEndpointTypeInterface,
		VpcId:            aws.String(w.cfg.Deployment.PrivateLinkVpcID),
		ServiceName:      aws.String(ep.EndpointService),
		SubnetIds:        w.cfg.Deployment.PrivateLinkSubnetIDs,
		SecurityGroupIds: []string{w.cfg.Deployment.PrivateLinkSGID},
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeVpcEndpoint,
			Tags: []ec2types.Tag{
				{Key: aws.String("astro.io/store-id"), Value: aws.String(storeID)},
				{Key: aws.String("astro.io/component"), Value: aws.String("knowledge")},
			},
		}},
	})
	if err != nil {
		w.setError(storeID, "CreateVpcEndpoint failed: "+err.Error())
		return fmt.Errorf("create vpc endpoint: %w", err)
	}

	vpceID := aws.ToString(out.VpcEndpoint.VpcEndpointId)
	if err := w.ksStore.SetEndpointVPCEID(storeID, vpceID); err != nil {
		w.log.Error("PrivateLinkProvision: failed to record VPCE ID", "error", err, "store_id", storeID)
	}
	if err := w.ksStore.SetEndpointStatus(storeID, knowledgestore.StatusPendingAcceptance); err != nil {
		w.log.Error("PrivateLinkProvision: failed to update endpoint status", "error", err, "store_id", storeID)
	}
	if err := w.ksStore.SetStatus(storeID, knowledgestore.StatusPendingAcceptance); err != nil {
		w.log.Error("PrivateLinkProvision: failed to update store status", "error", err, "store_id", storeID)
	}

	w.log.Info("PrivateLinkProvision: VPC endpoint created",
		"store_id", storeID, "vpce_id", vpceID, "service", ep.EndpointService)
	return nil
}

const (
	// AWS default limit — request an increase via Service Quotas if too low.
	maxVPCEndpointsPerVPC = 50
	vpceWarningThreshold  = 45
)

func (w *PrivateLinkProvisionWorker) checkVPCEndpointLimit(ctx context.Context, ec2Client knowledgestore.EC2Client) error {
	out, err := ec2Client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
		Filters: []ec2types.Filter{{
			Name:   aws.String("vpc-id"),
			Values: []string{w.cfg.Deployment.PrivateLinkVpcID},
		}},
	})
	if err != nil {
		w.log.Warn("PrivateLinkProvision: failed to check VPC endpoint count (proceeding anyway)", "error", err)
		return nil // non-fatal — let AWS reject it if over limit
	}

	count := len(out.VpcEndpoints)
	if count >= maxVPCEndpointsPerVPC {
		return fmt.Errorf("VPC endpoint limit reached: %d/%d interface endpoints in VPC %s — request an AWS limit increase via Service Quotas",
			count, maxVPCEndpointsPerVPC, w.cfg.Deployment.PrivateLinkVpcID)
	}
	if count >= vpceWarningThreshold {
		w.log.Warn("PrivateLinkProvision: approaching VPC endpoint limit",
			"count", count, "limit", maxVPCEndpointsPerVPC, "vpc_id", w.cfg.Deployment.PrivateLinkVpcID)
	}
	return nil
}

func (w *PrivateLinkProvisionWorker) setError(storeID, errMsg string) {
	if err := w.ksStore.SetEndpointError(storeID, errMsg); err != nil {
		w.log.Error("PrivateLinkProvision: failed to record endpoint error", "error", err, "store_id", storeID)
	}
	if err := w.ksStore.SetError(storeID, errMsg); err != nil {
		w.log.Error("PrivateLinkProvision: failed to record store error", "error", err, "store_id", storeID)
	}
}
