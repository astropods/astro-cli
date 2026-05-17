package deployer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployer handles K8s apply and teardown operations for deployments.
type Deployer struct {
	Registry     *k8s.Registry
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
	// KnowledgeStore for resolving bound knowledge entries (optional)
	KnowledgeStore *knowledgestore.Store
	// ImagePreflighter, when set, is plumbed into the Applier so the worker
	// re-checks tenant images against the registry. Defense-in-depth against
	// stale specs the handler couldn't catch.
	ImagePreflighter *k8s.ImagePreflighter
}

func (d *Deployer) clusterClient(ctx context.Context, dep *deploymentstore.Deployment) (k8s.ClusterClient, error) {
	if d == nil || d.Registry == nil {
		return nil, fmt.Errorf("deployer: k8s registry not configured")
	}
	if dep == nil {
		return nil, fmt.Errorf("deployer: nil deployment")
	}
	if dep.EffectiveClusterID() == "" {
		return d.Registry.Default(), nil
	}
	return d.Registry.Get(ctx, dep.EffectiveClusterID())
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
		d.Log.Info("Messaging OIDC not configured — MESSAGING_OIDC_ISSUER not set, deployments with auth:oidc will have no auth")
	} else if oidcAuth.ClientID == "" || oidcAuth.ClientSecret == "" {
		d.Log.Warn("Messaging OIDC misconfigured — MESSAGING_OIDC_CLIENT_ID or MESSAGING_OIDC_CLIENT_SECRET not set, auth:oidc will have no effect")
		oidcAuth = nil
	} else {
		d.Log.Info("Messaging OIDC configured", "issuer", oidcAuth.Issuer)
	}

	k8sForDep, err := d.clusterClient(ctx, dep)
	if err != nil {
		return nil, fmt.Errorf("resolve k8s client: %w", err)
	}

	// Resolve bound knowledge entries: look up store info and decrypt credentials.
	boundKnowledge, boundCredentials, boundErr := d.resolveBoundKnowledge(ctx, dep, &ds, k8sForDep)
	if boundErr != nil {
		return nil, fmt.Errorf("resolve bound knowledge: %w", boundErr)
	}

	applier := k8s.NewApplier(k8sForDep, k8s.ApplierConfig{
		Namespace:              dep.Namespace,
		RegistryURL:            d.Cfg.Deployment.RegistryURL,
		ProxyRegistryHost:      d.Cfg.Deployment.ProxyRegistryHost,
		Environment:            d.Cfg.Deployment.Environment,
		ImagePullPolicy:        imagePullPolicyForMode(d.Cfg.Deployment.K8sClientMode),
		ImagePreflighter:       d.ImagePreflighter,
		TenantImageHosts:       tenantImageHostsFromConfig(d.Cfg),
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
		BoundKnowledge:         boundKnowledge,
		BoundCredentials:       boundCredentials,
		DeployTokenSecret:      d.Cfg.Security.DeployTokenSecret,
		// The deploy token's iss claim carries this URL; the messaging
		// container reads it to know where to call back for authorize. We
		// reuse the public-facing frontend URL — the API surface is co-
		// located there, no separate authz endpoint.
		AuthzCallbackURL: d.Cfg.Auth.FrontendURL,
		NamespaceLabels:  buildNamespaceLabels(dep, acct.Name),
		NamespaceAnnotations: map[string]string{
			"astro.dev/display-name": dep.DisplayName,
		},
	})

	applyResult, applyErr := applier.ApplyDeploymentSpec(ctx, &ds)

	// On success, persist the resolved env to deployment_build_env.
	// Best-effort: failure here logs but does not fail the apply, since
	// the table is shadow-write today (the K8s applier still wires env
	// the old way; the rows are consumed by the API + UI). Once the
	// applier itself projects from these rows, this becomes load-bearing.
	if applyErr == nil && applyResult != nil && len(applyResult.Errors) == 0 {
		if err := d.populateBuildEnv(ctx, dep, &ds, boundKnowledge, applyResult.AllCredentials); err != nil {
			d.Log.Warn("Failed to populate deployment_build_env",
				"error", err, "deployment_id", dep.ID)
		}
	}

	return applyResult, applyErr
}

// populateBuildEnv runs deployment.Resolve over the rehydrated spec and
// writes the result to deployment_build_env. The rows are the source of
// truth the API + UI read from; the K8s applier keeps wiring env the
// old way until a follow-up flips it to projection from this table.
//
// Encryption uses the deployment's existing data key (decrypted via KMS)
// rather than minting a new one, so the rows stay readable across
// re-applies.
func (d *Deployer) populateBuildEnv(
	ctx context.Context,
	dep *deploymentstore.Deployment,
	ds *spec.AstroDeploymentSpec,
	boundKnowledge map[string]deployment.BoundKnowledgeInfo,
	allCredentials map[string]string,
) error {
	if d.Store == nil {
		return nil
	}

	enc, err := d.encryptorForDeployment(ctx, dep)
	if err != nil {
		return fmt.Errorf("encryptor: %w", err)
	}
	if enc == nil {
		// No KMS configured — skip writing for now. (Local dev / tests.)
		return nil
	}

	// account_var_ref provenance for user_var rows.
	storedVars, err := d.Store.GetDeploymentVariables(dep.ID)
	if err != nil {
		return fmt.Errorf("get variables: %w", err)
	}
	refs := make(map[string]string, len(storedVars))
	for _, v := range storedVars {
		if v.Ref != "" {
			refs[v.Name] = v.Ref
		}
	}

	externalAgentHost := ""
	if ep := spec.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
		if ep.Expose != nil && ep.Expose.Domain != "" {
			externalAgentHost = ep.Expose.Domain
		} else if d.Cfg.Deployment.IngressDomain != "" {
			externalAgentHost = k8s.GenerateIngressHost(dep.AgentName, dep.Namespace, d.Cfg.Deployment.IngressDomain)
		}
	}

	opts := deployment.ResolveOptions{
		Namespace:         dep.Namespace,
		BoundKnowledge:    boundKnowledge,
		BoundCredentials:  allCredentials,
		AccountVarRefs:    refs,
		AuthToken:         "", // applier owns token signing today; deployer doesn't see it
		LangfuseAuthToken: "", // ditto
		LangfuseBaseURL:   "",
		DeploymentID:      dep.ID,
		ExternalAgentHost: externalAgentHost,
	}
	rows, err := deployment.Resolve(ds, opts)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	writes := make([]deploymentstore.BuildEnvWrite, 0, len(rows))
	for _, r := range rows {
		writes = append(writes, deploymentstore.BuildEnvWrite{
			Role:          string(r.Role),
			EnvName:       r.EnvName,
			Value:         r.Value,
			IsSecret:      r.IsSecret,
			Source:        string(r.Source),
			UserVarName:   r.UserVarName,
			AccountVarRef: r.AccountVarRef,
			Optional:      r.Optional,
		})
	}
	return d.Store.SaveBuildEnv(dep.ID, writes, enc)
}

// encryptorForDeployment returns an Encryptor that uses the deployment's
// existing data key (decrypted via KMS), or nil if KMS isn't configured
// or the deployment has no encrypted data key.
func (d *Deployer) encryptorForDeployment(
	ctx context.Context,
	dep *deploymentstore.Deployment,
) (*envelope.Encryptor, error) {
	if len(dep.EncryptedDataKey) == 0 || d.Cfg.Deployment.KMSKeyARN == "" {
		return nil, nil
	}
	kmsClient := d.kmsClient(ctx)
	if kmsClient == nil {
		return nil, nil
	}
	out, err := kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: dep.EncryptedDataKey,
	})
	if err != nil {
		return nil, fmt.Errorf("kms decrypt data key: %w", err)
	}
	enc, err := envelope.NewEncryptorFromPlaintext(out.Plaintext, dep.EncryptedDataKey, d.Cfg.Deployment.KMSKeyARN)
	// Zero the plaintext key as soon as the gcm is built.
	for i := range out.Plaintext {
		out.Plaintext[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("build encryptor: %w", err)
	}
	return enc, nil
}

// resolveBoundKnowledge looks up store info and decrypts credentials for all bound
// knowledge entries in the deployment spec. Returns an error if any bound entry's
// store or credentials cannot be resolved — deploying without credentials would
// produce a running agent that silently fails to connect.
func (d *Deployer) resolveBoundKnowledge(
	ctx context.Context, dep *deploymentstore.Deployment, ds *spec.AstroDeploymentSpec, k8sForDep k8s.ClusterClient,
) (map[string]deployment.BoundKnowledgeInfo, map[string]string, error) {
	if d.KnowledgeStore == nil {
		return nil, nil, nil
	}

	var boundKnowledge map[string]deployment.BoundKnowledgeInfo
	var boundCredentials map[string]string

	for name, k := range ds.Knowledge {
		if !k.IsBound() {
			continue
		}
		store, err := d.KnowledgeStore.GetByARN(ctx, k.Binding)
		if err != nil {
			return nil, nil, fmt.Errorf("knowledge %q: failed to look up bound store %q: %w", name, k.Binding, err)
		}
		if store == nil {
			return nil, nil, fmt.Errorf("knowledge %q: bound store %q not found", name, k.Binding)
		}
		if boundKnowledge == nil {
			boundKnowledge = make(map[string]deployment.BoundKnowledgeInfo)
			boundCredentials = make(map[string]string)
		}
		storeNS := k8s.KnowledgeNamespace(store.AccountID)
		serviceName := k8s.KnowledgeResourceName(store.ID)
		boundKnowledge[name] = deployment.BoundKnowledgeInfo{
			Host:     deployment.GenerateServiceDNS(serviceName, storeNS),
			Provider: store.Provider,
		}

		// Resolve store credentials via unified resolver (KMS or k8s Secret fallback).
		creds, credErr := d.KnowledgeStore.GetCredentials(store.ID)
		if credErr != nil {
			return nil, nil, fmt.Errorf("knowledge %q: failed to get credentials for store %q: %w", name, store.ID, credErr)
		}
		plainCreds, resolveErr := knowledgestore.ResolveCredentials(
			ctx, store, creds, d.kmsClient(ctx),
			&k8s.KnowledgeSecretReader{Clientset: k8sForDep.Clientset()}, storeNS,
		)
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("knowledge %q: failed to resolve credentials for store %q: %w", name, store.ID, resolveErr)
		}
		storageKeyMap := spec.CredentialStorageKeyMap(store.Provider)
		for storageKey, val := range plainCreds {
			if attr, ok := storageKeyMap[storageKey]; ok {
				boundCredentials[name+"."+attr] = val
			}
		}
	}

	return boundKnowledge, boundCredentials, nil
}

// kmsClient returns the deployer's KMS client, or creates one from the default AWS config.
func (d *Deployer) kmsClient(ctx context.Context) envelope.KMSClient {
	if d.KMSClient != nil {
		return d.KMSClient
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		d.Log.Warn("Failed to load AWS config for KMS", "error", err)
		return nil
	}
	return kms.NewFromConfig(awsCfg)
}

// buildNamespaceLabels returns the namespace label set for a deployment.
// The source-account-id label is omitted when SourceAccountID is unset
// (legacy/ancient rows) so the reconciler's missing-label fallback
// path takes over rather than the row recording an empty-string label.
func buildNamespaceLabels(dep *deploymentstore.Deployment, accountName string) map[string]string {
	labels := map[string]string{
		"astro.dev/account-id":   dep.AccountID,
		"astro.dev/account":      accountName,
		deployment.LabelKeyAgent: dep.AgentName,
		"astro.dev/build":        dep.BuildID,
	}
	if dep.SourceAccountID != nil && *dep.SourceAccountID != "" {
		labels[deployment.LabelKeySourceAccountID] = *dep.SourceAccountID
	}
	return labels
}

// Teardown deletes the K8s namespace for a deployment, cascading to all resources.
// Returns nil if the namespace is already gone (idempotent).
func (d *Deployer) Teardown(ctx context.Context, dep *deploymentstore.Deployment) error {
	k8sForDep, err := d.clusterClient(ctx, dep)
	if err != nil {
		return fmt.Errorf("resolve k8s client: %w", err)
	}
	err = k8sForDep.Clientset().CoreV1().Namespaces().Delete(
		ctx, dep.Namespace, metav1.DeleteOptions{},
	)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// RehydrateSecrets loads user-variable values from the
// deployment_build_env user_var rows and injects them back into the
// spec. Values are encrypted with the deployment's data key (every row,
// secret or not — the schema is uniform); we decrypt via KMS when
// configured.
//
// Multiple rows can share a user_var_name (a variable with
// Targets=["agent","ingestion"] produces one row per role). Their
// values are identical by construction, so we take the first one we
// see and skip duplicates.
func (d *Deployer) RehydrateSecrets(ctx context.Context, dep *deploymentstore.Deployment, ds *spec.AstroDeploymentSpec) error {
	rows, err := d.Store.GetBuildEnv(dep.ID)
	if err != nil {
		return fmt.Errorf("get deployment_build_env: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	// Build a decryptor if the deployment has KMS-encrypted secrets.
	var dec *envelope.Decryptor
	if len(dep.EncryptedDataKey) > 0 && d.Cfg.Deployment.KMSKeyARN != "" {
		if kmsClient := d.kmsClient(ctx); kmsClient != nil {
			dec, err = envelope.NewDecryptor(ctx, kmsClient, dep.EncryptedDataKey)
			if err != nil {
				d.Log.Warn("Failed to create KMS decryptor", "error", err, "deployment_id", dep.ID)
			}
		}
	}

	if ds.Variables == nil {
		ds.Variables = make(map[string]spec.Variable)
	}

	seen := map[string]bool{}
	for _, r := range rows {
		if r.Source != "user_var" || r.UserVarName == "" {
			continue
		}
		if seen[r.UserVarName] {
			continue
		}
		seen[r.UserVarName] = true

		existing, ok := ds.Variables[r.UserVarName]
		if !ok {
			continue
		}

		var val string
		if dec != nil && len(r.Nonce) > 0 {
			plaintext, decErr := dec.Decrypt(r.ValueEncrypted, r.Nonce)
			if decErr != nil {
				d.Log.Warn("Failed to decrypt variable", "name", r.UserVarName, "deployment_id", dep.ID)
				continue
			}
			val = string(plaintext)
		} else {
			// No KMS configured — plaintext stored in value_encrypted.
			val = string(r.ValueEncrypted)
		}

		existing.Value = val
		ds.Variables[r.UserVarName] = existing
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

// tenantImageHostsFromConfig returns the registry hostnames whose images are
// "ours" — i.e. images we should HEAD-preflight before deploy. Skipping
// non-tenant hosts (docker.io, quay.io, gcr.io, ...) avoids issuing wasted
// HEAD requests + spurious 401/403s for every public sidecar image.
//
// In local mode the convention is "registry.<server-host>" (see astro-cli
// auth.RegistryURLFromServerURL), which after a typical local setup ends up
// at "registry.localhost". We hardcode that fallback rather than reading
// from a non-existent config field.
func tenantImageHostsFromConfig(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	hosts := make([]string, 0, 3)
	if h := stripScheme(cfg.Deployment.ProxyRegistryHost); h != "" {
		hosts = append(hosts, h)
	}
	if h := stripScheme(cfg.Deployment.RegistryURL); h != "" {
		hosts = append(hosts, h)
	}
	if cfg.Deployment.K8sClientMode == "local" {
		hosts = append(hosts, "registry.localhost")
	}
	return hosts
}

// stripScheme returns the host portion of "scheme://host[/path]" inputs;
// inputs without a scheme pass through (after trimming any trailing path).
func stripScheme(s string) string {
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "://"); i > 0 {
		s = s[i+3:]
	}
	if i := strings.Index(s, "/"); i > 0 {
		s = s[:i]
	}
	return s
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
