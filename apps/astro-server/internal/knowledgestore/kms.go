package knowledgestore

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSBackend returns the envelope KMS backend for knowledge-store credential
// encryption and decryption. The choice is made purely on environment:
// ENVIRONMENT == "local" uses the compiled-in local dev backend; every other
// environment uses real AWS KMS. Selection is never inferred from the stored
// key ARN — the same credential logic runs everywhere, only the backend behind
// the envelope.KMSClient interface differs.
func KMSBackend(ctx context.Context, localMode bool) (envelope.KMSClient, error) {
	if localMode {
		return envelope.NewLocalKMSClient()
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return kms.NewFromConfig(awsCfg), nil
}
