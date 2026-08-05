package deployer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/clustercfg"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrClusterClientUnavailable means Teardown hit a permanent cluster-client
// resolution failure (cluster missing/disabled, IAM deny). Callers may proceed
// with DB-only cleanup. Transient resolution errors are returned unwrapped so
// undeploy workers retry.
var ErrClusterClientUnavailable = errors.New("cluster client unavailable")

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
	// AI Gateway per-account virtual key provisioning (optional). Nil when
	// AI_GATEWAY_URL is unset — deploys that reference provider:astro-gateway
	// are rejected by the validator before reaching the applier.
	AIGatewayStore       *aigateway.Store
	AIGatewayProvisioner *aigateway.Provisioner
	// KnowledgeStore for resolving bound knowledge entries (optional)
	KnowledgeStore *knowledgestore.Store
	// ImagePreflighter, when set, is plumbed into the Applier so the worker
	// re-checks tenant images against the registry. Defense-in-depth against
	// stale specs the handler couldn't catch.
	ImagePreflighter *k8s.ImagePreflighter
}

func (d *Deployer) clusterClient(ctx context.Context, dep *deploymentstore.Deployment) (k8s.ClusterClient, error) {
	if dep == nil {
		return nil, fmt.Errorf("deployer: nil deployment")
	}
	return d.clusterClientForKey(ctx, dep.EffectiveClusterID())
}

func (d *Deployer) clusterClientForKey(ctx context.Context, clusterID string) (k8s.ClusterClient, error) {
	if d == nil || d.Registry == nil {
		return nil, fmt.Errorf("deployer: k8s registry not configured")
	}
	if clusterID == "" {
		return d.Registry.Default(), nil
	}
	return d.Registry.Get(ctx, clusterID)
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

	var ds deployment.AstroDeploymentSpec
	if err := json.Unmarshal(rev.SpecJSON, &ds); err != nil {
		return nil, fmt.Errorf("unmarshal deployment spec: %w", err)
	}

	// Reconstitute secret variable values from the normalized store.
	// The revision spec has secrets stripped; the actual values live in
	// deployment_build_env (potentially KMS-encrypted).
	if err := d.RehydrateSecrets(ctx, dep, &ds); err != nil {
		return nil, fmt.Errorf("rehydrate secrets: %w", err)
	}

	// Look up account name for namespace labels
	acct, err := d.AccountStore.GetByID(dep.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account %s: %w", dep.AccountID, err)
	}

	// In local mode there's no ingress to inject an OIDC identity header
	// into messaging. Resolve the account's first member and pin them as
	// the fixed authn identity. The lookup is best-effort — if it fails
	// the messaging container falls back to NoopSessionManager.
	var authTestUserID string
	if d.Cfg.Deployment.K8sClientMode == "local" {
		if uid, uErr := d.AccountStore.GetFirstMemberUserID(dep.AccountID); uErr != nil {
			d.Log.Warn("Local-mode authn user lookup failed; messaging will run unauthenticated",
				"error", uErr, "account_id", dep.AccountID)
		} else {
			authTestUserID = uid
		}
	}

	// Resolve cluster client + config up front — the cluster's Langfuse URL
	// flows into both the collector env and the AI Gateway key metadata,
	// so we need it before either provisioner call.
	k8sForDep, err := d.clusterClient(ctx, dep)
	if err != nil {
		return nil, fmt.Errorf("resolve k8s client: %w", err)
	}

	clusterCfg, err := clustercfg.Resolve(ctx, d.Registry, d.Cfg.Deployment, dep.EffectiveClusterID())
	if err != nil {
		return nil, fmt.Errorf("resolve cluster config: %w", err)
	}

	// Langfuse: ensure per-account project exists and compute the collector
	// auth token. Per-tenant pk/sk are not embedded into AI Gateway key
	// metadata — gateway-side traces go through the collector path, not via
	// LiteLLM's Langfuse callback.
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

	// AI Gateway: mint a per-deployment virtual key if the spec opts in via
	// agent.astro_ai_gateway: true. Rotate-on-redeploy — each call to Apply mints
	// a fresh key and demotes the previous to a short-lived prev slot.
	// Fail-hard: an agent without its primary credential is a broken agent.
	var aigwAPIKey, aigwBaseURL string
	if ds.Agent.AIGateway {
		if d.AIGatewayProvisioner == nil || d.AIGatewayStore == nil {
			return nil, fmt.Errorf("agent.astro_ai_gateway is true but AI Gateway is not enabled in this environment (AI_GATEWAY_URL unset)")
		}
		apiKey, baseURL, agErr := d.AIGatewayProvisioner.EnsureDeploymentKey(
			ctx, d.AIGatewayStore,
			d.Cfg.Deployment.KMSKeyARN, d.kmsClient(ctx),
			aigateway.DeploymentKeyParams{
				AccountID:    acct.ID,
				DeploymentID: dep.ID,
				ClusterID:    dep.EffectiveClusterID(),
				AgentName:    ds.Source.Name,
				AgentVersion: ds.Source.Build,
			},
		)
		if agErr != nil {
			return nil, fmt.Errorf("ensure ai-gateway key for deployment %s: %w", dep.ID, agErr)
		}
		aigwAPIKey, aigwBaseURL = apiKey, baseURL
	}

	// Messaging OIDC is enforced at the front-door ALB listener rule (managed
	// in astro-infra), not per-deployment. No per-tenant OIDC config to build
	// here; see docs/plans/tenant-router-migration.md.

	// Resolve bound knowledge entries: look up store info and decrypt credentials.
	boundKnowledge, boundCredentials, boundErr := d.resolveBoundKnowledge(ctx, dep, &ds)
	if boundErr != nil {
		return nil, fmt.Errorf("resolve bound knowledge: %w", boundErr)
	}

	applier := k8s.NewApplier(k8sForDep, k8s.ApplierConfig{
		Namespace:                dep.Namespace,
		ProxyRegistryHost:        d.Cfg.Deployment.ProxyRegistryHost,
		RegistryPullCredential:   d.Cfg.Deployment.RegistryPullCredential,
		ImagePullPolicy:          imagePullPolicyForMode(d.Cfg.Deployment.K8sClientMode),
		ImagePreflighter:         d.ImagePreflighter,
		TenantImageHosts:         tenantImageHostsFromConfig(d.Cfg),
		IngressDomain:            clusterCfg.AgentIngressDomain,
		AgentPublicIngressDomain: clusterCfg.AgentPublicIngressDomain,
		IngestionIngressDomain:   clusterCfg.IngestionIngressDomain,
		LangfuseAuthToken:        langfuseAuthToken,
		LangfuseBaseURL:          clusterCfg.LangfuseBaseURL,
		DeploymentID:             dep.ID,
		PodSubnetCIDRs:           clusterCfg.PodSubnetCIDRs,
		CPSubnetCIDRs:            clusterCfg.CPSubnetCIDRs,
		LangfuseVPCEIPs:          clusterCfg.LangfuseVPCEIPs,
		LocalMode:                d.Cfg.Deployment.K8sClientMode == "local",
		AstroGatewayAPIKey:       aigwAPIKey,
		AstroGatewayBaseURL:      aigwBaseURL,
		BoundKnowledge:           boundKnowledge,
		BoundCredentials:         boundCredentials,
		DeployTokenSecret:        d.Cfg.Security.DeployTokenSecret,
		// The deploy token's iss claim carries this URL; the messaging
		// container reads it to know where to call back for authorize. We
		// reuse the public-facing frontend URL — the API surface is co-
		// located there, no separate authz endpoint. In local mode the
		// frontend URL is a host-side localhost address that pods can't
		// reach via their own loopback, so rewrite to host.docker.internal.
		AuthzCallbackURL: podReachableURL(d.Cfg.Auth.FrontendURL, d.Cfg.Deployment.K8sClientMode == "local"),
		AuthTestUserID:   authTestUserID,
		PersistMessagingHost: func(depID, host string) error {
			return d.Store.UpdateMessagingIngressHost(depID, host)
		},
		NamespaceLabels: buildNamespaceLabels(dep, acct.Name),
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
	ds *deployment.AstroDeploymentSpec,
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
	// enc may be nil here when KMS isn't configured (local dev / tests).
	// SaveBuildEnv still proceeds — non-secret rows are written plaintext,
	// secret rows fall back to plaintext too. This keeps the table populated
	// so the runtime endpoint can surface env on local deployments instead
	// of returning a blank list.

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
	if ep := deployment.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
		if ep.Expose != nil && ep.Expose.Domain != "" {
			externalAgentHost = ep.Expose.Domain
		} else {
			ingressCfg, resolveErr := clustercfg.Resolve(ctx, d.Registry, d.Cfg.Deployment, dep.EffectiveClusterID())
			if resolveErr != nil {
				return fmt.Errorf("resolve cluster ingress config: %w", resolveErr)
			}
			if ingressCfg.AgentIngressDomain != "" {
				externalAgentHost = k8s.GenerateIngressHost(dep.AgentName, dep.Namespace, ingressCfg.AgentIngressDomain)
			}
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
	ctx context.Context, dep *deploymentstore.Deployment, ds *deployment.AstroDeploymentSpec,
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
		creds, credErr := d.KnowledgeStore.GetCredentials(store.ID)
		if credErr != nil {
			return nil, nil, fmt.Errorf("knowledge %q: failed to get credentials for store %q: %w", name, store.ID, credErr)
		}
		plainCreds, resolveErr := knowledgestore.ResolveCredentials(ctx, store, creds, d.kmsClient(ctx))
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("knowledge %q: failed to resolve credentials for store %q: %w", name, store.ID, resolveErr)
		}

		// Stores reached over PrivateLink must dial the provisioned VPC
		// endpoint's DNS — the user-supplied host is a vpce-svc service name that
		// isn't itself connectable. Fetch the endpoint record so the host
		// resolver can pick it up.
		ep, err := d.KnowledgeStore.GetEndpoint(store.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("knowledge %q: failed to look up endpoint for store %q: %w", name, store.ID, err)
		}

		host, hostErr := boundKnowledgeHost(store, ep, plainCreds)
		if hostErr != nil {
			return nil, nil, fmt.Errorf("knowledge %q: %w", name, hostErr)
		}
		boundKnowledge[name] = deployment.BoundKnowledgeInfo{
			Host:     host,
			Provider: store.Provider,
		}

		for attr, val := range mapBoundCredentials(plainCreds) {
			boundCredentials[name+"."+attr] = val
		}
	}

	return boundKnowledge, boundCredentials, nil
}

// boundKnowledgeHost returns the host the agent should use to reach a bound
// knowledge store.
//
//   - Stores reached over PrivateLink: the provisioned VPC endpoint DNS. The
//     user-supplied host is a "com.amazonaws.vpce.*" service name that is not
//     itself connectable, so the endpoint's resolved DNS is the only usable host.
//   - Directly reachable stores: the user-supplied HOST credential.
func boundKnowledgeHost(store *knowledgestore.KnowledgeStore, ep *knowledgestore.Endpoint, creds map[string]string) (string, error) {
	if ep != nil && ep.EndpointDNS != nil && *ep.EndpointDNS != "" {
		return *ep.EndpointDNS, nil
	}
	if host := creds["HOST"]; host != "" {
		return host, nil
	}
	return "", fmt.Errorf("store %q has no resolvable host: no PrivateLink endpoint DNS and no HOST credential", store.Name)
}

// credKeyToAttr maps the generic credential keys stores are stored under (set
// at connect time) to bind attribute names.
var credKeyToAttr = map[string]string{
	"USERNAME": "user",
	"PASSWORD": "password",
	"DATABASE": "database",
	"API_KEY":  "api_key",
}

// mapBoundCredentials translates a store's plaintext credentials into the
// attr→value form the resolver consumes ("name.attr"). HOST/PORT are connection
// coords, not credentials, and are intentionally dropped here.
func mapBoundCredentials(plainCreds map[string]string) map[string]string {
	out := make(map[string]string, len(plainCreds))
	for key, val := range plainCreds {
		if attr, ok := credKeyToAttr[key]; ok {
			out[attr] = val
		}
	}
	return out
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

// Teardown deletes the K8s namespace for a deployment on its routing cluster.
// Returns nil if the namespace is already gone (idempotent).
//
// Also revokes the deployment's AI Gateway virtual key upstream. The DB row
// would otherwise be cleaned up by the ON DELETE CASCADE on the deployments
// FK, but LiteLLM has no FK back to us — without the explicit revoke the
// upstream key would orphan and keep accruing /key/list rows.
func (d *Deployer) Teardown(ctx context.Context, dep *deploymentstore.Deployment) error {
	if dep != nil && d.AIGatewayProvisioner != nil && d.AIGatewayStore != nil {
		if err := d.AIGatewayProvisioner.RevokeDeploymentKey(ctx, d.AIGatewayStore, dep.ID); err != nil {
			// Best-effort: log and continue. A failed upstream revoke is
			// not a reason to block namespace deletion — the row stays put
			// and the next purge sweep retries.
			d.Log.Warn("Failed to revoke AI Gateway key during teardown", "error", err, "deployment_id", dep.ID)
		}
	}
	return d.TeardownOnCluster(ctx, dep, dep.EffectiveClusterID())
}

// TeardownOnCluster deletes the K8s namespace on an explicit cluster, regardless
// of deployments.cluster_id. Used for cross-cluster migration before routing updates.
func (d *Deployer) TeardownOnCluster(ctx context.Context, dep *deploymentstore.Deployment, clusterID string) error {
	if dep == nil {
		return fmt.Errorf("deployer: nil deployment")
	}
	k8sForDep, err := d.clusterClientForKey(ctx, clusterID)
	if err != nil {
		if k8s.IsPermanentClientResolutionError(err) {
			return fmt.Errorf("%w: resolve k8s client: %w", ErrClusterClientUnavailable, err)
		}
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
func (d *Deployer) RehydrateSecrets(ctx context.Context, dep *deploymentstore.Deployment, ds *deployment.AstroDeploymentSpec) error {
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
		ds.Variables = make(map[string]deployment.Variable)
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

		plaintext, decErr := dec.Decrypt(r.ValueEncrypted, r.Nonce)
		if decErr != nil {
			d.Log.Warn("Failed to decrypt variable", "name", r.UserVarName, "deployment_id", dep.ID)
			continue
		}
		existing.Value = string(plaintext)
		ds.Variables[r.UserVarName] = existing
	}

	return nil
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

// podReachableURL rewrites a host-side localhost URL so it's reachable from
// a pod's network namespace on Docker Desktop's k8s. From inside a pod,
// `localhost` is the pod's own loopback, not the host running astro-server;
// `host.docker.internal` is the Docker Desktop alias for the macOS host.
// Only rewrites in local mode and only when the host is loopback — other
// hosts (e.g. a Service DNS name or a real ingress) are left alone.
func podReachableURL(raw string, localMode bool) string {
	if !localMode || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return raw
	}
	if p := u.Port(); p != "" {
		u.Host = "host.docker.internal:" + p
	} else {
		u.Host = "host.docker.internal"
	}
	return u.String()
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
