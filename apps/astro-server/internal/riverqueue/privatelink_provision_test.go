package riverqueue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
)

// fakeEC2Client is a test double for the subnet-selection path. Only the two
// describe calls are exercised; the rest satisfy the EC2Client interface.
type fakeEC2Client struct {
	knowledgestore.EC2Client

	subnetAZ          map[string]string // subnet ID → AZ name returned by DescribeSubnets
	serviceAZs        []string          // AZs returned for the queried service
	serviceNotFound   bool              // DescribeVpcEndpointServices returns no ServiceDetails
	describeSvcErr    error
	describeSubnetErr error

	describeSubnetsCalls int
}

func (f *fakeEC2Client) DescribeVpcEndpointServices(_ context.Context, _ *ec2.DescribeVpcEndpointServicesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointServicesOutput, error) {
	if f.describeSvcErr != nil {
		return nil, f.describeSvcErr
	}
	if f.serviceNotFound {
		return &ec2.DescribeVpcEndpointServicesOutput{}, nil
	}
	return &ec2.DescribeVpcEndpointServicesOutput{
		ServiceDetails: []ec2types.ServiceDetail{{AvailabilityZones: f.serviceAZs}},
	}, nil
}

func (f *fakeEC2Client) DescribeSubnets(_ context.Context, _ *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	f.describeSubnetsCalls++
	if f.describeSubnetErr != nil {
		return nil, f.describeSubnetErr
	}
	subnets := make([]ec2types.Subnet, 0, len(f.subnetAZ))
	for id, az := range f.subnetAZ {
		subnets = append(subnets, ec2types.Subnet{
			SubnetId:         aws.String(id),
			AvailabilityZone: aws.String(az),
		})
	}
	return &ec2.DescribeSubnetsOutput{Subnets: subnets}, nil
}

func newProvisionWorker(subnetIDs []string) *PrivateLinkProvisionWorker {
	return &PrivateLinkProvisionWorker{
		cfg: &config.Config{
			Deployment: config.DeploymentConfig{PrivateLinkSubnetIDs: subnetIDs},
		},
	}
}

func TestSelectSubnetsForService_SubsetOfAZs(t *testing.T) {
	// Six subnets, one per AZ. Service is only published in three of them.
	subnetIDs := []string{"subnet-a", "subnet-b", "subnet-c", "subnet-d", "subnet-e", "subnet-f"}
	fake := &fakeEC2Client{
		subnetAZ: map[string]string{
			"subnet-a": "us-east-1a",
			"subnet-b": "us-east-1b",
			"subnet-c": "us-east-1c",
			"subnet-d": "us-east-1d",
			"subnet-e": "us-east-1e",
			"subnet-f": "us-east-1f",
		},
		serviceAZs: []string{"us-east-1b", "us-east-1d", "us-east-1f"},
	}

	w := newProvisionWorker(subnetIDs)
	got, err := w.selectSubnetsForService(context.Background(), fake, "com.amazonaws.vpce.us-east-1.svc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"subnet-b", "subnet-d", "subnet-f"}
	if !equalStrings(got, want) {
		t.Fatalf("selected subnets = %v, want %v (must preserve config order)", got, want)
	}
}

func TestSelectSubnetsForService_NoOverlap(t *testing.T) {
	subnetIDs := []string{"subnet-a", "subnet-b"}
	fake := &fakeEC2Client{
		subnetAZ: map[string]string{
			"subnet-a": "us-east-1a",
			"subnet-b": "us-east-1b",
		},
		serviceAZs: []string{"us-east-1e", "us-east-1f"}, // disjoint from our subnets
	}

	w := newProvisionWorker(subnetIDs)
	_, err := w.selectSubnetsForService(context.Background(), fake, "com.amazonaws.vpce.us-east-1.svc-xyz")
	if err == nil {
		t.Fatal("expected fast-fail error on empty AZ intersection, got nil")
	}

	// Error must be actionable: name the service and both AZ sets.
	msg := err.Error()
	for _, want := range []string{"svc-xyz", "not available in any AZ", "us-east-1e", "us-east-1a"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestResolveSubnetAZs_CachesDescribeSubnets(t *testing.T) {
	fake := &fakeEC2Client{
		subnetAZ:   map[string]string{"subnet-a": "us-east-1a"},
		serviceAZs: []string{"us-east-1a"},
	}
	w := newProvisionWorker([]string{"subnet-a"})

	for i := range 3 {
		if _, err := w.selectSubnetsForService(context.Background(), fake, "svc"); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if fake.describeSubnetsCalls != 1 {
		t.Fatalf("DescribeSubnets called %d times, want 1 (result should be cached)", fake.describeSubnetsCalls)
	}
}

func TestSelectSubnetsForService_ServiceNotFound(t *testing.T) {
	fake := &fakeEC2Client{serviceNotFound: true}
	w := newProvisionWorker([]string{"subnet-a"})

	_, err := w.selectSubnetsForService(context.Background(), fake, "svc-missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestSelectSubnetsForService_DescribeServicesError(t *testing.T) {
	fake := &fakeEC2Client{describeSvcErr: errors.New("boom")}
	w := newProvisionWorker([]string{"subnet-a"})

	if _, err := w.selectSubnetsForService(context.Background(), fake, "svc"); err == nil {
		t.Fatal("expected error from DescribeVpcEndpointServices, got nil")
	}
}

func TestSelectSubnetsForService_AllAZsSupported(t *testing.T) {
	subnetIDs := []string{"subnet-a", "subnet-b", "subnet-c"}
	fake := &fakeEC2Client{
		subnetAZ: map[string]string{
			"subnet-a": "us-east-1a",
			"subnet-b": "us-east-1b",
			"subnet-c": "us-east-1c",
		},
		serviceAZs: []string{"us-east-1a", "us-east-1b", "us-east-1c"},
	}

	w := newProvisionWorker(subnetIDs)
	got, err := w.selectSubnetsForService(context.Background(), fake, "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalStrings(got, subnetIDs) {
		t.Fatalf("got %v, want all subnets %v", got, subnetIDs)
	}
}

func TestSelectSubnetsForService_SingleAZService(t *testing.T) {
	subnetIDs := []string{"subnet-a", "subnet-b", "subnet-c"}
	fake := &fakeEC2Client{
		subnetAZ: map[string]string{
			"subnet-a": "us-east-1a",
			"subnet-b": "us-east-1b",
			"subnet-c": "us-east-1c",
		},
		serviceAZs: []string{"us-east-1b"},
	}

	w := newProvisionWorker(subnetIDs)
	got, err := w.selectSubnetsForService(context.Background(), fake, "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalStrings(got, []string{"subnet-b"}) {
		t.Fatalf("got %v, want [subnet-b]", got)
	}
}

func TestSelectSubnetsForService_DescribeSubnetsError(t *testing.T) {
	fake := &fakeEC2Client{
		serviceAZs:        []string{"us-east-1a"},
		describeSubnetErr: errors.New("throttled"),
	}
	w := newProvisionWorker([]string{"subnet-a"})

	if _, err := w.selectSubnetsForService(context.Background(), fake, "svc"); err == nil {
		t.Fatal("expected error from DescribeSubnets, got nil")
	}
}

// A subnet that DescribeSubnets did not return (e.g. removed) has no AZ mapping
// and must be excluded rather than silently treated as matching.
func TestFilterSubnetsByServiceAZ_UnknownSubnetExcluded(t *testing.T) {
	subnetIDs := []string{"subnet-a", "subnet-ghost", "subnet-b"}
	subnetAZ := map[string]string{
		"subnet-a": "us-east-1a",
		"subnet-b": "us-east-1b",
		// subnet-ghost intentionally absent
	}
	got := filterSubnetsByServiceAZ(subnetIDs, subnetAZ, []string{"us-east-1a", "us-east-1b"})
	if !equalStrings(got, []string{"subnet-a", "subnet-b"}) {
		t.Fatalf("got %v, want [subnet-a subnet-b] (ghost excluded)", got)
	}
}

func TestFilterSubnetsByServiceAZ_PreservesOrder(t *testing.T) {
	// Service AZ order differs from subnet order; output must follow subnet order.
	subnetIDs := []string{"subnet-1", "subnet-2", "subnet-3", "subnet-4"}
	subnetAZ := map[string]string{
		"subnet-1": "az1",
		"subnet-2": "az2",
		"subnet-3": "az3",
		"subnet-4": "az4",
	}
	got := filterSubnetsByServiceAZ(subnetIDs, subnetAZ, []string{"az4", "az1", "az3"})
	if !equalStrings(got, []string{"subnet-1", "subnet-3", "subnet-4"}) {
		t.Fatalf("got %v, want subnet order [subnet-1 subnet-3 subnet-4]", got)
	}
}

func TestFilterSubnetsByServiceAZ_EmptyServiceAZs(t *testing.T) {
	got := filterSubnetsByServiceAZ([]string{"subnet-a"}, map[string]string{"subnet-a": "az1"}, nil)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty when service publishes no AZs", got)
	}
}

func TestSortedAZValues_DedupesAndSorts(t *testing.T) {
	// Two subnets share an AZ; an empty AZ is dropped.
	subnetAZ := map[string]string{
		"subnet-a": "us-east-1c",
		"subnet-b": "us-east-1a",
		"subnet-c": "us-east-1c",
		"subnet-d": "",
	}
	got := sortedAZValues(subnetAZ)
	want := []string{"us-east-1a", "us-east-1c"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSortedStrings_DoesNotMutateInput(t *testing.T) {
	in := []string{"c", "a", "b"}
	got := sortedStrings(in)
	if !equalStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %v, want sorted", got)
	}
	if !equalStrings(in, []string{"c", "a", "b"}) {
		t.Fatalf("input was mutated: %v", in)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
