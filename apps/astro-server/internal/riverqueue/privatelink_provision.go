package riverqueue

import (
	"context"
	"fmt"
	"sort"
	"sync"

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

func init() {
	registerJobKind[PrivateLinkProvisionArgs]()
}

// PrivateLinkProvisionWorker creates AWS VPC endpoints for external knowledge stores.
type PrivateLinkProvisionWorker struct {
	river.WorkerDefaults[PrivateLinkProvisionArgs]
	ksStore *knowledgestore.Store
	cfg     *config.Config
	log     *logger.Logger

	// subnetAZ caches the managed-VPC subnet → AZ-name mapping. A subnet's AZ
	// never changes, so we resolve it once via DescribeSubnets and reuse it for
	// every endpoint we provision.
	subnetAZMu sync.Mutex
	subnetAZ   map[string]string
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

	// AWS rejects CreateVpcEndpoint outright if any passed subnet is in an AZ the
	// target service is not published in. Select only the subnets whose AZ the
	// service actually supports.
	subnetIDs, err := w.selectSubnetsForService(ctx, ec2Client, ep.EndpointService)
	if err != nil {
		w.setError(storeID, err.Error())
		return err
	}

	out, err := ec2Client.CreateVpcEndpoint(ctx, &ec2.CreateVpcEndpointInput{
		VpcEndpointType:  ec2types.VpcEndpointTypeInterface,
		VpcId:            aws.String(w.cfg.Deployment.PrivateLinkVpcID),
		ServiceName:      aws.String(ep.EndpointService),
		SubnetIds:        subnetIDs,
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

// selectSubnetsForService returns the subset of PRIVATELINK_SUBNET_IDS whose AZ
// the target endpoint service is published in. The returned slice preserves the
// configured subnet order. It fails fast with an actionable error if the service
// is not available in any AZ our subnets live in, rather than letting AWS reject
// the CreateVpcEndpoint call with a raw InvalidParameter error.
func (w *PrivateLinkProvisionWorker) selectSubnetsForService(ctx context.Context, ec2Client knowledgestore.EC2Client, serviceName string) ([]string, error) {
	svcOut, err := ec2Client.DescribeVpcEndpointServices(ctx, &ec2.DescribeVpcEndpointServicesInput{
		ServiceNames: []string{serviceName},
	})
	if err != nil {
		return nil, fmt.Errorf("describe vpc endpoint services for %s: %w", serviceName, err)
	}
	if len(svcOut.ServiceDetails) == 0 {
		return nil, fmt.Errorf("knowledge-store service %s not found", serviceName)
	}
	// AvailabilityZones are returned mapped into the consumer (our) account's AZ
	// naming, so they're directly comparable to our subnets' AZ names.
	serviceAZs := svcOut.ServiceDetails[0].AvailabilityZones

	subnetAZ, err := w.resolveSubnetAZs(ctx, ec2Client)
	if err != nil {
		return nil, err
	}

	matched := filterSubnetsByServiceAZ(w.cfg.Deployment.PrivateLinkSubnetIDs, subnetAZ, serviceAZs)
	if len(matched) == 0 {
		return nil, fmt.Errorf(
			"knowledge-store service %s is not available in any AZ of the managed VPC (service AZs: %v, available subnet AZs: %v)",
			serviceName, sortedStrings(serviceAZs), sortedAZValues(subnetAZ))
	}
	return matched, nil
}

// resolveSubnetAZs returns (and caches) the managed-VPC subnet → AZ-name mapping.
// One DescribeSubnets call covers all subnets; the result never changes.
func (w *PrivateLinkProvisionWorker) resolveSubnetAZs(ctx context.Context, ec2Client knowledgestore.EC2Client) (map[string]string, error) {
	w.subnetAZMu.Lock()
	defer w.subnetAZMu.Unlock()
	if w.subnetAZ != nil {
		return w.subnetAZ, nil
	}

	out, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: w.cfg.Deployment.PrivateLinkSubnetIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("describe subnets: %w", err)
	}

	m := make(map[string]string, len(out.Subnets))
	for _, s := range out.Subnets {
		m[aws.ToString(s.SubnetId)] = aws.ToString(s.AvailabilityZone)
	}
	w.subnetAZ = m
	return m, nil
}

// filterSubnetsByServiceAZ returns the subnetIDs (in their original order) whose
// AZ name appears in the service's supported AZ set.
func filterSubnetsByServiceAZ(subnetIDs []string, subnetAZ map[string]string, serviceAZs []string) []string {
	azSet := make(map[string]struct{}, len(serviceAZs))
	for _, az := range serviceAZs {
		azSet[az] = struct{}{}
	}
	var matched []string
	for _, id := range subnetIDs {
		if _, ok := azSet[subnetAZ[id]]; ok {
			matched = append(matched, id)
		}
	}
	return matched
}

// sortedAZValues returns the unique, sorted AZ names from a subnet → AZ map.
func sortedAZValues(subnetAZ map[string]string) []string {
	seen := make(map[string]struct{}, len(subnetAZ))
	for _, az := range subnetAZ {
		if az != "" {
			seen[az] = struct{}{}
		}
	}
	azs := make([]string, 0, len(seen))
	for az := range seen {
		azs = append(azs, az)
	}
	sort.Strings(azs)
	return azs
}

// sortedStrings returns a sorted copy of s without mutating the input.
func sortedStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}

func (w *PrivateLinkProvisionWorker) setError(storeID, errMsg string) {
	if err := w.ksStore.SetEndpointError(storeID, errMsg); err != nil {
		w.log.Error("PrivateLinkProvision: failed to record endpoint error", "error", err, "store_id", storeID)
	}
	if err := w.ksStore.SetError(storeID, errMsg); err != nil {
		w.log.Error("PrivateLinkProvision: failed to record store error", "error", err, "store_id", storeID)
	}
}
