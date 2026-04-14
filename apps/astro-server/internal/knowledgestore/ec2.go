package knowledgestore

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// EC2Client is the subset of the EC2 API used for PrivateLink automation.
type EC2Client interface {
	CreateVpcEndpoint(ctx context.Context, params *ec2.CreateVpcEndpointInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcEndpointOutput, error)
	DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error)
	DeleteVpcEndpoints(ctx context.Context, params *ec2.DeleteVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcEndpointsOutput, error)
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	GetManagedPrefixListEntries(ctx context.Context, params *ec2.GetManagedPrefixListEntriesInput, optFns ...func(*ec2.Options)) (*ec2.GetManagedPrefixListEntriesOutput, error)
	ModifyManagedPrefixList(ctx context.Context, params *ec2.ModifyManagedPrefixListInput, optFns ...func(*ec2.Options)) (*ec2.ModifyManagedPrefixListOutput, error)
	DescribeManagedPrefixLists(ctx context.Context, params *ec2.DescribeManagedPrefixListsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeManagedPrefixListsOutput, error)
}

// NewEC2Client loads the default AWS config and returns an EC2 client.
func NewEC2Client(ctx context.Context) (EC2Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return ec2.NewFromConfig(cfg), nil
}
