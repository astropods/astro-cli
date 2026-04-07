package deployer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployer handles K8s apply and teardown operations for deployments.
type Deployer struct {
	K8sClient    k8s.ClusterClient
	AccountStore *account.AccountStore
	Cfg          *config.Config
	Store        *deploymentstore.Store
	Log          *logger.Logger
	// KMSClient is an optional KMS client for decrypting secrets. If nil,
	// a client is created from the default AWS config at decrypt time.
	KMSClient envelope.KMSClient
	// Langfuse per-account project provisioning (optional)
	LangfuseStore       *langfuse.Store
	LangfuseProvisioner *langfuse.Provisioner
}

// Apply provisions K8s resources for a deployment using the current revision's spec.
func (d *Deployer) Apply(ctx context.Context, dep *deploymentstore.Deployment) (*k8s.ApplyResult, error) {
	// Load spec from current revision
	rev, err := d.Store.GetCurrentRevision(dep.ID)
	if err != nil {
		return nil, fmt.Errorf("get current revision: %w", err)
	}
	if rev == nil {
		return nil, fmt.Errorf("no current revision for deployment %s", dep.ID)
	}

	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal(rev.SpecJSON, &ds); err != nil {
		return nil, fmt.Errorf("unmarshal deployment spec: %w", err)
	}

	// Reconstitute secret variable values from the normalized store.
	// The revision spec has secrets stripped; the actual values live in
	// deployment_variables (potentially KMS-encrypted).
	if err := d.RehydrateSecrets(ctx, dep, &ds); err != nil {
		return nil, fmt.Errorf("rehydrate secrets: %w", err)
	}

	// Look up account name for namespace labels
	acct, err := d.AccountStore.GetByID(dep.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account %s: %w", dep.AccountID, err)
	}

	// Langfuse: ensure per-account project exists and compute auth token
	var langfuseAuthToken string
	if d.LangfuseProvisioner != nil && d.LangfuseStore != nil {
		pk, sk, lfErr := d.LangfuseProvisioner.EnsureProject(
			ctx, d.LangfuseStore,
			d.Cfg.Deployment.KMSKeyARN, d.KMSClient,
			acct.ID, acct.Name,
		)
		if lfErr != nil {
			d.Log.Warn("Langfuse provisioning failed, continuing without", "error", lfErr, "account", acct.Name)
		} else {
			langfuseAuthToken = base64.StdEncoding.EncodeToString([]byte(pk + ":" + sk))
		}
	}

	oidcAuth := messagingOIDCAuthFromConfig(d.Cfg)
	if oidcAuth == nil {
		d.Log.Info("Messaging OIDC auth disabled — MESSAGING_OIDC_ISSUER not set, ingress will be unauthenticated")
	} else if oidcAuth.ClientID == "" || oidcAuth.ClientSecret == "" {
		d.Log.Warn("Messaging OIDC auth misconfigured — MESSAGING_OIDC_CLIENT_ID or MESSAGING_OIDC_CLIENT_SECRET not set, skipping OIDC annotations")
		oidcAuth = nil
	} else {
		d.Log.Info("Messaging OIDC auth enabled", "issuer", oidcAuth.Issuer)
	}

	applier := k8s.NewApplier(d.K8sClient, k8s.ApplierConfig{
		Namespace:              dep.Namespace,
		RegistryURL:            d.Cfg.Deployment.RegistryURL,
		ProxyRegistryHost:      d.Cfg.Deployment.ProxyRegistryHost,
		Environment:            d.Cfg.Deployment.Environment,
		ImagePullPolicy:        imagePullPolicyForMode(d.Cfg.Deployment.K8sClientMode),
		IngressDomain:          d.Cfg.Deployment.IngressDomain,
		ACMCertificateARN:      d.Cfg.Deployment.ACMCertificateARN,
		ALBGroupName:           d.Cfg.Deployment.ALBGroupName,
		IngestionIngressDomain: d.Cfg.Deployment.IngestionIngressDomain,
		IngestionACMCertARN:    d.Cfg.Deployment.IngestionACMCertARN,
		IngestionALBGroupName:  d.Cfg.Deployment.IngestionALBGroupName,
		LangfuseAuthToken:      langfuseAuthToken,
		LangfuseBaseURL:        langfuseBaseURLForCollector(d.Cfg),
		DeploymentID:           dep.ID,
		PodSubnetCIDRs:         d.Cfg.Deployment.PodSubnetCIDRs,
		LangfuseVPCEIPs:        d.Cfg.Deployment.LangfuseVPCEIPs,
		LocalMode:              d.Cfg.Deployment.K8sClientMode == "local",
		ManagedAnthropicAPIKey: d.Cfg.Deployment.ManagedAnthropicAPIKey,
		MessagingOIDCAuth:      oidcAuth,
		NamespaceLabels: map[string]string{
			"astro.dev/account-id": dep.AccountID,
			"astro.dev/account":    acct.Name,
			"astro.dev/agent":      dep.AgentName,
			"astro.dev/build":      dep.BuildID,
		},
		NamespaceAnnotations: map[string]string{
			"astro.dev/display-name": dep.DisplayName,
		},
	})

	return applier.ApplyDeploymentSpec(ctx, &ds)
}

// Teardown deletes the K8s namespace for a deployment, cascading to all resources.
// Returns nil if the namespace is already gone (idempotent).
func (d *Deployer) Teardown(ctx context.Context, dep *deploymentstore.Deployment) error {
	err := d.K8sClient.Clientset().CoreV1().Namespaces().Delete(
		ctx, dep.Namespace, metav1.DeleteOptions{},
	)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// RehydrateSecrets loads secret variable values from the deployment_variables
// table and injects them back into the spec. Values may be KMS-encrypted;
// if so they are decrypted using the deployment's encrypted data key.
func (d *Deployer) RehydrateSecrets(ctx context.Context, dep *deploymentstore.Deployment, ds *spec.AstroDeploymentSpec) error {
	storedVars, err := d.Store.GetDeploymentVariables(dep.ID)
	if err != nil {
		return fmt.Errorf("get deployment variables: %w", err)
	}
	if len(storedVars) == 0 {
		return nil
	}

	// Build a decryptor if the deployment has KMS-encrypted secrets
	var dec *envelope.Decryptor
	if len(dep.EncryptedDataKey) > 0 && d.Cfg.Deployment.KMSKeyARN != "" {
		kmsClient := d.KMSClient
		if kmsClient == nil {
			awsCfg, awsErr := awsconfig.LoadDefaultConfig(ctx)
			if awsErr != nil {
				d.Log.Warn("Failed to load AWS config for secret decryption", "error", awsErr, "deployment_id", dep.ID)
			} else {
				kmsClient = kms.NewFromConfig(awsCfg)
			}
		}
		if kmsClient != nil {
			dec, err = envelope.NewDecryptor(ctx, kmsClient, dep.EncryptedDataKey)
			if err != nil {
				d.Log.Warn("Failed to create KMS decryptor", "error", err, "deployment_id", dep.ID)
			}
		}
	}

	if ds.Variables == nil {
		ds.Variables = make(map[string]spec.Variable)
	}

	for _, sv := range storedVars {
		existing, ok := ds.Variables[sv.Name]
		if !ok {
			continue
		}

		val := sv.Value
		if sv.Secret && dec != nil && len(sv.Nonce) > 0 {
			ciphertext, b64Err := base64.StdEncoding.DecodeString(val)
			if b64Err == nil {
				plaintext, decErr := dec.Decrypt(ciphertext, sv.Nonce)
				if decErr == nil {
					val = string(plaintext)
				} else {
					d.Log.Warn("Failed to decrypt variable", "name", sv.Name, "deployment_id", dep.ID)
				}
			}
		}

		existing.Value = val
		ds.Variables[sv.Name] = existing
	}

	return nil
}

// langfuseBaseURLForCollector returns the Langfuse URL that the collector
// sidecar should use. In production the collector may need an external URL
// (LANGFUSE_BASE_URL_EXT) different from the internal one the server uses.
func langfuseBaseURLForCollector(cfg *config.Config) string {
	if cfg.Deployment.LangfuseBaseURLExt != "" {
		return cfg.Deployment.LangfuseBaseURLExt
	}
	return cfg.Deployment.LangfuseBaseURL
}

func imagePullPolicyForMode(mode string) corev1.PullPolicy {
	if mode == "local" {
		return corev1.PullIfNotPresent
	}
	return corev1.PullAlways
}

// messagingOIDCAuthFromConfig builds an OIDCAuthConfig from server config.
// Returns nil if the issuer is not set (OIDC disabled).
func messagingOIDCAuthFromConfig(cfg *config.Config) *k8s.OIDCAuthConfig {
	d := cfg.Deployment
	if d.MessagingOIDCIssuer == "" {
		return nil
	}
	return &k8s.OIDCAuthConfig{
		Issuer:                d.MessagingOIDCIssuer,
		AuthorizationEndpoint: d.MessagingOIDCAuthEndpoint,
		TokenEndpoint:         d.MessagingOIDCTokenEndpoint,
		UserInfoEndpoint:      d.MessagingOIDCUserInfoEndpoint,
		ClientID:              d.MessagingOIDCClientID,
		ClientSecret:          d.MessagingOIDCClientSecret,
		SessionTimeoutSeconds: d.MessagingOIDCSessionTimeout,
	}
}
