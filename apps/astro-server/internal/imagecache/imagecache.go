// Package imagecache force-refreshes ECR Docker Hub pull-through cache images.
//
// Agents pull sidecar images (e.g. messaging) from the account's ECR Docker Hub
// pull-through cache. ECR only re-checks Docker Hub for a given tag at most once
// every ~24h. Deleting the cached tag evicts it, so the next agent pull is
// treated as a fresh import from Docker Hub — bypassing that window.
package imagecache

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// Messaging pull-through cache coordinates. The deployment template resolves the
// messaging sidecar to "{ecrHost}/dockerhub/astropods/messaging:latest"; the ECR
// API addresses that as repository "dockerhub/astropods/messaging", tag "latest".
const (
	MessagingCacheRepo = "dockerhub/astropods/messaging"
	MessagingCacheTag  = "latest"
)

// ecrAPI is the subset of the ECR client used here.
type ecrAPI interface {
	BatchDeleteImage(ctx context.Context, params *ecr.BatchDeleteImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error)
}

// Refresher evicts pull-through cache tags via the ECR API using the pod's IRSA
// credentials.
type Refresher struct {
	region string
	ecr    ecrAPI // nil → created lazily from AWS config
}

// New returns a Refresher targeting the given AWS region. An empty region lets
// the AWS SDK resolve it from the environment (IRSA / AWS_REGION).
func New(region string) *Refresher {
	return &Refresher{region: region}
}

func (r *Refresher) client(ctx context.Context) (ecrAPI, error) {
	if r.ecr != nil {
		return r.ecr, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(r.region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return ecr.NewFromConfig(cfg), nil
}

// RefreshMessaging deletes the messaging pull-through cache tag so the next agent
// pull re-imports it from Docker Hub. A missing repository or tag is treated as
// success (nothing cached → next pull is already fresh). Returns the targeted
// "repo:tag".
func (r *Refresher) RefreshMessaging(ctx context.Context) (string, error) {
	target := MessagingCacheRepo + ":" + MessagingCacheTag

	client, err := r.client(ctx)
	if err != nil {
		return target, err
	}

	repo := MessagingCacheRepo
	out, err := client.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
		RepositoryName: &repo,
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: aws.String(MessagingCacheTag)}},
	})
	if err != nil {
		// No cache repo yet → next pull imports fresh; nothing to evict.
		var notFound *ecrtypes.RepositoryNotFoundException
		if errors.As(err, &notFound) {
			return target, nil
		}
		return target, fmt.Errorf("batch delete image %s: %w", target, err)
	}

	// BatchDeleteImage reports per-image problems as Failures, not as a call
	// error. A missing tag just means it was already evicted.
	for _, f := range out.Failures {
		if f.FailureCode == ecrtypes.ImageFailureCodeImageNotFound {
			continue
		}
		return target, fmt.Errorf("delete %s failed: %s (%s)", target, aws.ToString(f.FailureReason), f.FailureCode)
	}

	return target, nil
}
