package envelope

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

type Vault struct {
	client KMSClient
	keyARN string
}

func Open(ctx context.Context, localMode bool, keyARN string) (*Vault, error) {
	if localMode {
		client, err := NewLocalKMSClient()
		if err != nil {
			return nil, fmt.Errorf("envelope: local backend: %w", err)
		}
		return &Vault{client: client, keyARN: keyARN}, nil
	}
	if keyARN == "" {
		return nil, fmt.Errorf("envelope: KMS_KEY_ARN is required outside local mode")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("envelope: load aws config: %w", err)
	}
	return &Vault{client: kms.NewFromConfig(awsCfg), keyARN: keyARN}, nil
}

func NewVault(client KMSClient, keyARN string) *Vault {
	return &Vault{client: client, keyARN: keyARN}
}

func (v *Vault) Encryptor(ctx context.Context) (*Encryptor, error) {
	if v == nil {
		return nil, errNoVault
	}
	return NewEncryptor(ctx, v.client, v.keyARN)
}

func (v *Vault) EncryptorFor(ctx context.Context, encryptedDataKey []byte) (*Encryptor, error) {
	if v == nil {
		return nil, errNoVault
	}
	plaintextKey, err := DecryptDataKey(ctx, v.client, encryptedDataKey)
	if err != nil {
		return nil, err
	}
	defer zero(plaintextKey)
	return NewEncryptorFromPlaintext(plaintextKey, encryptedDataKey, v.keyARN)
}

func (v *Vault) Decryptor(ctx context.Context, encryptedDataKey []byte) (*Decryptor, error) {
	if v == nil {
		return nil, errNoVault
	}
	return NewDecryptor(ctx, v.client, encryptedDataKey)
}

var errNoVault = fmt.Errorf("envelope: no vault configured")

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
