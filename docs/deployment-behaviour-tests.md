# Deployment Behaviour Test Coverage

End-to-end test cases covering the full deployment pipeline: spec parsing, template generation, env/secret resolution, K8s resource projection.

Legend: ✓ test exists | ✗ missing

---

## 1. Deployment Template Generation

Tests parse an inline `astropods.yml` spec via `spec.ParseString` and assert the generated `AstroDeploymentSpec` has the correct shape.

### A. Interfaces / Adapters

| # | Test Case | Status |
|---|-----------|--------|
| A1 | No `interfaces` key → messaging enabled by default, no slack variables | ✓ `TestYAML_Interfaces_A1_DefaultNoSlackVars` |
| A2 | `interfaces.messaging: true` → same result as default, no slack variables | ✓ `TestYAML_Interfaces_A2_MessagingExplicitTrueNoSlackVars` |
| A3 | `interfaces.messaging: false` → interfaces block nil, no slack variables | ✓ `TestYAML_Interfaces_A3_MessagingDisabled` |
| A4 | `interfaces.frontend: true` (messaging omitted) → agent port 80 with expose enabled, no messaging block | ✓ `TestYAML_Interfaces_A4_FrontendOnly` |
| A5 | `interfaces.frontend: true, messaging: true` → agent port 80 with expose AND messaging block present | ✓ `TestYAML_Interfaces_A5_FrontendAndMessaging` |
| A6 | `interfaces.frontend: false, messaging: false` explicitly → no interfaces block, agent stays on port 8080 | ✓ `TestYAML_Interfaces_A6_BothExplicitlyFalse` |
| A7 | Messaging enabled → all interfaces block defaults: empty adapters, image resolved, grpc port 9090, http port 8080 not exposed, MessagingResources, auth.web.type=oidc, auth.slack nil | ✓ `TestYAML_Interfaces_A7_AllDefaults` |
| A8 | Frontend + messaging both enabled → interfaces block fields identical to A7 | ✓ `TestYAML_Interfaces_A8_FrontendAndMessagingInterfacesBlock` |
| A9 | Adapter shaping is a no-op when the spec has no interfaces block | ✓ `TestApplyAdapterShaping_NilInterfaces` |
| A10 | When the template already has stored adapters and the request carries no override, the response reflects the stored selection | ✓ `TestShapeTemplate_AdaptersFromPrefill` |
| A11 | User can change the adapter selection mid-flow; clearing all adapters returns an empty slice (not null) | ✓ `TestShapeTemplate_AdaptersReshape` |
| A12 | Stripping only removes variables targeted exclusively at non-selected adapters — agent-targeted vars are never touched | ✓ `TestApplyAdapterShaping_KeepsSelectedAndNonInterface` |
| A13 | Deploy round-trip with web-only: deploy handler regenerates template, shapes to match, and `EnforceEditable` passes | ✓ `TestApplyAdapterShaping_DeployRoundTrip` |
| A14 | Deploy round-trip with slack selected: deploy handler shapes the fresh template and `EnforceEditable` passes with tokens filled | ✓ `TestApplyAdapterShaping_DeployRoundTripSlackSelected` |

### B. Slack Configuration

Slack variables are never in the raw generated template. They appear only once the user picks the slack adapter — the server injects them at shaping time. The `dev` block is never read server-side.

| # | Test Case | Status |
|---|-----------|--------|
| B1 | Selecting slack injects `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` as required, `SLACK_CONFIG` as optional, and wires all three into `interfaces.environment` so the messaging container can read them at runtime | ✓ `TestApplyAdapterShaping_SlackOptionalityFlipped` / `TestTemplate_SlackConfigVariable_WithSpecConfig` |
| B2 | Switching away from slack removes all three vars and their `interfaces.environment` refs | ✓ `TestApplyAdapterShaping_CleansEnvironmentRefs` |
| B3 | Re-opening a web-only deployment (no adapter override in the request) still strips any slack vars that were previously present | ✓ `TestShapeTemplate_NoRequestInterfaces_StripsNonSelectedAdapterRefs` |
| B4 | If the user pre-defined `SLACK_BOT_TOKEN`/`SLACK_APP_TOKEN` in their spec inputs, their default values survive when shaping overwrites the platform metadata | ✓ `TestGenerateDeploymentTemplate_SlackInputDefaultsPreserved` |
| B5 | Selecting slack via `ShapeTemplate` sets adapters in the response, marks tokens required, and fires validation errors when tokens are missing | ✓ `TestShapeTemplate_AdapterShaping` |
| B6 | Re-templating a web-only deployment (admingrpc path) does not leak slack vars into the resolved ConfigMap or Secret | ✓ `TestRetemplate_SlackConfigNotLeakedForWebOnly` |
| B7 | Deploying with slack selected and both tokens filled → no validation errors | ✓ `TestTemplateDeploy_SlackAdapter_WithTokens` |
| B8 | Deploying with slack selected but a token missing → required-field validation error for that token | ✓ `TestTemplateDeploy_SlackAdapter_MissingBotToken` / `TestTemplateDeploy_SlackAdapter_MissingAppToken` / `TestTemplateDeploy_SlackAdapter_MissingBothTokens` |

### C. Knowledge Providers

#### Template generation

| # | Test Case | Status |
|---|-----------|--------|
| C1 | A postgres store gets the right image, listens on port 5432, and the agent receives host/port env refs | ✓ `TestTemplate_ProviderKnowledge_Postgres` |
| C2 | Marking a postgres store as persistent gives it 10Gi storage and a recreate update strategy | ✓ `TestTemplate_ProviderKnowledge_Postgres_Persistent` |
| C3 | Postgres credentials (USER, PASSWORD, DB) are not in the agent's environment — they flow via secretKeyRef at apply time | ✓ `TestTemplate_MultiplePostgresKnowledge_Credentials` |
| C4 | A redis store gets the right image, an exec healthcheck, and the agent receives host/port/URL env refs; the password is not in agent env | ✓ `TestTemplate_ProviderKnowledge_Redis` |
| C5 | A persistent qdrant store gets the right image, a health path, 10Gi storage, and a recreate update strategy | ✓ `TestTemplate_ProviderKnowledge_Qdrant` |
| C6 | A non-persistent qdrant store gets no storage config and a rolling update strategy | ✓ `TestTemplate_KnowledgeNonPersistent_NoStorage` |
| C7 | A neo4j store exposes both the HTTP and bolt ports and seeds `NEO4J_AUTH=none` in its container env | ✓ `TestTemplate_ProviderKnowledge_Neo4j` |
| C8 | A cloud knowledge provider (pinecone) produces no sidecar container — only an API key variable wired to the agent | ✓ `TestTemplate_ProviderKnowledge_Pinecone` |
| C9 | A custom-container knowledge store has its image resolved and uses the `KNOWLEDGE_<NAME>_*` env prefix instead of a provider prefix | ✓ `TestTemplate_ContainerKnowledge` |
| C10 | Two postgres stores get disambiguated env keys (`POSTGRES_PRIMARY_HOST`, `POSTGRES_SECONDARY_HOST`); the bare `POSTGRES_HOST` key is not emitted | ✓ `TestTemplate_MultiplePostgresKnowledge_Credentials` |
| C11 | Postgres, redis, and qdrant together each wire their own host ref into the agent env without collision | ✓ `TestTemplate_MultipleKnowledgeProviders` |

#### K8s apply

| # | Test Case | Status |
|---|-----------|--------|
| C12 | A persistent knowledge store is applied as a StatefulSet with a PVC | ✓ `TestApplyDeploymentSpec_WithKnowledgePersistent` |
| C13 | A non-persistent knowledge store is applied as a Deployment, not a StatefulSet | ✓ `TestApplyDeploymentSpec_WithKnowledgeNonPersistent` |
| C14 | The agent container receives secretKeyRef entries for the knowledge store's credentials | ✓ `TestApplyDeploymentSpec_KnowledgeCredSecretKeyRefs_Agent` |
| C15 | Ingestion containers also receive secretKeyRef entries for knowledge store credentials | ✓ `TestApplyDeploymentSpec_KnowledgeCredSecretKeyRefs_Ingestion` |

#### Managed store bindings

| # | Test Case | Status |
|---|-----------|--------|
| C16 | Binding a knowledge entry to a managed store zeroes its container fields, drops its credential variables, and removes its editable fields — the deploy round-trip then passes `EnforceEditable` | ✓ `TestApplyBindingShaping_DeployRoundTrip` |
| C17 | Binding shaping does nothing when no knowledge entries are bound | ✓ `TestApplyBindingShaping_NoBoundEntries` |
| C18 | When re-deploying, bound entries are restored from the stored spec so the user does not have to re-select the managed store | ✓ `TestRestoreBindingsFromSpec` |
| C19 | Restoring bindings gracefully returns nothing when there are no bound entries, the JSON is empty, or the JSON is invalid | ✓ `TestRestoreBindingsFromSpec_NoBoundEntries` / `TestRestoreBindingsFromSpec_EmptyJSON` / `TestRestoreBindingsFromSpec_InvalidJSON` |

### D. Model Providers

| # | Test Case | Status |
|---|-----------|--------|
| D1 | Container-mode model (`models.<name>.container`) → sidecar Deployment created, `MODEL_<NAME>_HOST/PORT/URL` wired to agent env | ✓ container-mode model tests |
| D2 | `anthropic` (cloud) → no container in spec, `ANTHROPIC_API_KEY` secret variable wired to agent env | ✓ `TestTemplate_VariablesFromCloudProviders` |
| D3 | `openai` (cloud) → no container, `OPENAI_API_KEY` secret variable wired to agent env | ✓ `TestTemplate_ProviderModel_OpenAI` |
| D4 | `google` (cloud) → no container, `GOOGLE_API_KEY` secret variable wired to agent env | ✓ `TestTemplate_ProviderModel_Google` |
| D5 | `cohere` (cloud) → no container, `COHERE_API_KEY` secret variable wired to agent env | ✓ `TestTemplate_ProviderModel_Cohere` |
| D6 | `anthropic-managed` → no container, no credential variable created at all | ✓ `TestTemplate_ManagedProviderNoVariable` |
| D7 | Two cloud providers together → both credential variables present, both wired to agent env | ✓ `TestTemplate_ProviderModel_MultipleCloudProviders` |
| D8 | Container-mode model and cloud provider together → model container deployed, anthropic produces credential only | ✓ container-mode + cloud tests |

### E. Ingestion

| # | Test Case | Status |
|---|-----------|--------|
| E1 | Schedule trigger → `trigger.type=schedule`, cron field empty (user fills at deploy time) | ✓ `TestTemplate_IngestionSchedule` |
| E2 | Webhook trigger with port → port wired into endpoints | ✓ `TestTemplate_IngestionWebhookPort` |
| E3 | Startup trigger → `trigger.type=startup`, no port required | ✓ `TestTemplate_IngestionAllTypes` |
| E4 | Multiple ingestions (schedule + webhook + startup) → all three present in template | ✓ `TestTemplate_IngestionAllTypes` |

### F. Inputs / Variables

#### Component inputs

| # | Test Case | Status |
|---|-----------|--------|
| F1 | Top-level `inputs` → variable in template with description, default, and optional preserved; target is `["agent","ingestion"]` | ✓ `TestTemplate_TopLevelInputs_WiredAsVariableRefs` |
| F2 | Non-secret top-level input → wired to `agent.environment` as `${variables.KEY}` ref | ✓ `TestTemplate_TopLevelInputs_WiredAsVariableRefs` |
| F3 | Secret top-level input → appears in variables but NOT in `agent.environment` | ✓ `TestTemplate_TopLevelInputs_SecretNotInAgentEnv` |
| F4 | Agent component `inputs` → variable with target `["agent"]`; non-secret wired to agent env, secret not | ✓ `TestTemplate_AgentComponentInputs` |
| F5 | Model component `inputs` with a default → value injected directly into the model container's environment, not into the variables map | ✓ `TestTemplate_ModelComponentInputs` |
| F6 | Knowledge component `inputs` with a default → value injected directly into the knowledge container's environment, not into the variables map | ✓ `TestTemplate_KnowledgeComponentInputs` |
| F7 | Ingestion component `inputs` → variable with target `["ingestion.<name>"]` and default also wired into the ingestion container's environment | ✓ `TestTemplate_IngestionComponentInputs` |

#### Custom providers

| # | Test Case | Status |
|---|-----------|--------|
| F8 | Custom provider variables use `{PROVIDER}_{SUFFIX}` naming and respect the optional flag | ✓ `TestTemplate_VariablesCustomProvider` |
| F9 | Custom provider with an integration: all variables present, descriptions preserved, and each one wired into agent env | ✓ `TestTemplate_JiraIntegrationInputs` |

#### Variable naming and collisions

| # | Test Case | Status |
|---|-----------|--------|
| F10 | A single provider entry uses the bare prefix key (`ANTHROPIC_API_KEY`, not `CLAUDE_ANTHROPIC_API_KEY`) | ✓ `TestTemplate_NameDerivedVariableKeys` |
| F11 | When a user input collides with a provider credential, the user's default value wins | ✓ `TestGenerateDeploymentTemplate_CredentialInputDefaultMerge` |

#### ShapeTemplate — variable operations

| # | Test Case | Status |
|---|-----------|--------|
| F12 | Submitting a value via ShapeTemplate fills it in both the template and the root schema | ✓ `TestShapeTemplate_VariableFilling` |
| F13 | Submitting a variable ref (account variable) via ShapeTemplate clears the value field — value and ref are mutually exclusive | ✓ `TestShapeTemplate_VariableRef` |

### G. End-to-End

| # | Test Case | Status |
|---|-----------|--------|
| G1 | Full spec: all cloud model providers (anthropic, openai, google, cohere), a container-mode model, all knowledge providers (postgres, redis, qdrant, neo4j, pinecone), all ingestion trigger types (schedule, webhook, startup), a custom provider, inputs at agent and top-level, a custom container integration, messaging enabled, and a managed store binding — all components correct, cloud providers produce no containers, bound store is zeroed with ARN preserved, unbound stores untouched, all `${...}` env refs parse and resolve | ✓ `TestTemplate_FullSpec` |

---

## 2. Env Reference Resolution (ResolveDeploymentSpecEnv)

Takes the filled deployment spec and resolves every `${...}` reference in `agent.environment` and `interfaces.environment` into concrete values, splitting results into `ConfigMapData` (non-secret) and `SecretData` (secret).

| # | Test Case | Status |
|---|-----------|--------|
| E1 | Model host/port/URL refs resolve to the service DNS name and port in ConfigMapData | ✓ `TestResolveDeploymentSpecEnv_ModelReferences` |
| E2 | Knowledge host/port refs resolve to the knowledge service DNS name | ✓ `TestResolveDeploymentSpecEnv_KnowledgeReferences` |
| E3 | Integration URL refs resolve to the integration service DNS name and port | ✓ `TestResolveDeploymentSpecEnv_ToolReferences` |
| E4 | Variable ref targeting a secret lands in SecretData; targeting a non-secret lands in ConfigMapData | ✓ `TestResolveDeploymentSpecEnv_VariableReferences` |
| E5 | Source refs (`${source.name}`, `${source.build}`, `${source.account}`) resolve to the deployment source fields | ✓ `TestResolveDeploymentSpecEnv_SourceReferences` |
| E6 | Composite refs (multiple `${...}` in one string) resolve correctly | ✓ `TestResolveDeploymentSpecEnv_CompositeReferences` |
| E7 | Plain (non-ref) values pass through unchanged into ConfigMapData | ✓ `TestResolveDeploymentSpecEnv_PlainValues` |
| E8 | Platform vars (`ASTRO_AGENT_NAME`, `ASTRO_AGENT_BUILD`, `ASTRO_AGENT_URL`, `OTEL_EXPORTER_OTLP_ENDPOINT`) are always present | ✓ `TestResolveDeploymentSpecEnv_PlatformVars` |
| E9 | Custom observability port is reflected in `OTEL_EXPORTER_OTLP_ENDPOINT` | ✓ `TestResolveDeploymentSpecEnv_OTELCustomPort` |
| E10 | Default observability port (4318) used when none is set | ✓ `TestResolveDeploymentSpecEnv_OTELDefaultPort` |
| E11 | Observability container env vars are included in the resolved output | ✓ `TestResolveDeploymentSpecEnv_ObservabilityEnv` |
| E12 | Messaging sidecar GRPC address resolved into agent env when interfaces are configured | ✓ `TestResolveDeploymentSpecEnv_InterfacesGRPCAddr` |
| E13 | Custom messaging port overrides the default | ✓ `TestResolveDeploymentSpecEnv_InterfacesCustomPort` |
| E14 | Default messaging port (9090) used when none is set | ✓ `TestResolveDeploymentSpecEnv_InterfacesDefaultPort` |
| E15 | Interface env variable refs (e.g. `${variables.SLACK_BOT_TOKEN}`) resolve into messaging SecretData | ✓ `TestResolveDeploymentSpecEnv_InterfaceEnvVariableRefs` |
| E16 | Integration secret credentials land in SecretData, not ConfigMapData | ✓ `TestResolveDeploymentSpecEnv_JiraIntegrationSecrets` |
| E17 | Composite value containing a secret ref is routed to SecretData | ✓ `TestResolveDeploymentSpecEnv_CompositeSecretRef` |
| E18 | Hardcoded env value whose key matches a secret variable name goes to SecretData | ✓ `TestResolveDeploymentSpecEnv_HardcodedValueMatchingSecretKey` |
| E19 | Empty variables map produces no SecretData entries | ✓ `TestResolveDeploymentSpecEnv_EmptyVariables` |
| E20 | Empty non-secret variable resolves to an empty string in ConfigMapData | ✓ `TestResolveDeploymentSpecEnv_EmptyNonSecretVariableResolvesToEmptyString` |
| E21 | Stripped secret (value empty after resolve) is excluded from SecretData | ✓ `TestResolveDeploymentSpecEnv_StrippedSecretRouting` |
| E22 | Keys produced for a re-templated deployment match those from a fresh deploy | ✓ `TestResolveDeploymentSpecEnv_BackfillKeySetMatchesFreshDeploy` |
| E23 | Knowledge credential refs present in each knowledge container's resolved env | ✓ `TestResolveDeploymentSpecEnv_KnowledgeCredentialRefs` |

---

## 3. Persisting a Deployment (SaveNormalizedSpec)

Takes the resolved deployment spec and writes it into the normalised DB tables. Each component becomes a `deployment_workloads` row (or `deployment_sidecars` for the messaging container), each endpoint becomes a `deployment_services` row, persistent stores get a `deployment_volumes` row, and every user variable fans out to one `deployment_build_env` row per resolved role.

#### Workloads and services

| # | Test Case | Status |
|---|-----------|--------|
| S1 | Full spec: all workload types correct together (Deployment vs StatefulSet, image, replicas, trigger) | ✓ `TestSaveNormalizedSpec_FullSpec_Workloads` |
| S1a | Agent produces a Deployment workload | ✓ `TestSaveNormalizedSpec_S1_AgentIsDeployment` |
| S1b | Non-persistent knowledge produces a Deployment | ✓ `TestSaveNormalizedSpec_S1_NonPersistentKnowledgeIsDeployment` |
| S1c | Persistent knowledge produces a StatefulSet | ✓ `TestSaveNormalizedSpec_S1_PersistentKnowledgeIsStatefulSet` |
| S1d | Ingestion trigger type and schedule stored on the workload row | ✓ `TestSaveNormalizedSpec_S1_IngestionTriggerTypeStored` |
| S1e | Collector produces a Deployment workload | ✓ `TestSaveNormalizedSpec_S1_CollectorIsDeployment` |
| S2 | Persistent knowledge and GPU model each get a volume row; non-persistent does not | ✓ `TestSaveNormalizedSpec_FullSpec_Workloads` |
| S2a | Persistent knowledge gets a volume row | ✓ `TestSaveNormalizedSpec_S2_PersistentKnowledgeGetsVolume` |
| S2b | Non-persistent knowledge has no volume row | ✓ `TestSaveNormalizedSpec_S2_NonPersistentKnowledgeHasNoVolume` |
| S3 | Each workload gets a service row per declared endpoint | ✓ `TestSaveNormalizedSpec_FullSpec_Workloads` / `TestSaveNormalizedSpec_S3_ServiceCreatedPerEndpoint` |
| S4 | When observability is enabled, a collector workload row is created with the correct image and resources | ✓ `TestSaveNormalizedSpec_S1_CollectorIsDeployment` / `TestSaveNormalizedSpec_CollectorWorkloadCreated` |
| S5 | When observability is disabled, no collector workload row is created | ✓ `TestSaveNormalizedSpec_NoCollectorWorkloadWhenDisabled` |
| S6 | When an adapter is selected, a messaging sidecar row is created in deployment_sidecars | ✓ `TestSaveNormalizedSpec_MessagingSidecarCreated` |
| S7 | When no adapter is selected, no messaging sidecar row is created | ✓ `TestSaveNormalizedSpec_NoMessagingSidecarWithoutAdapters` |

#### Variable fan-out into deployment_build_env

| # | Test Case | Status |
|---|-----------|--------|
| S4 | Non-secret variable stored as plaintext bytes, nil nonce | ✓ `TestSaveNormalizedSpec_WithEncryptor` |
| S5 | Secret variable stored as ciphertext + nonce (KMS path) | ✓ `TestSaveNormalizedSpec_EncryptedBase64` |
| S6 | Secret variable stored as plaintext bytes when KMS absent (nil nonce) | ✓ `TestSaveNormalizedSpec_EncryptedBase64` |
| S7 | Variable with empty targets produces no rows | ✓ `TestSaveNormalizedSpec_EmptyTargets_DoesNotDiverge` |
| S8 | All variable targeting scenarios together | ✓ `TestSaveNormalizedSpec_BuildEnv_Targeting` |
| S8a | `targets: ["agent"]` → one row with role='agent', absent from messaging | ✓ `TestSaveNormalizedSpec_S8_AgentTarget` |
| S9 | `targets: ["interface.slack"]` → one row with role='messaging', absent from agent | ✓ `TestSaveNormalizedSpec_S9_MessagingTarget` |
| S10 | `targets: ["ingestion"]` → one row per declared ingestion (fan-out), absent from agent | ✓ `TestSaveNormalizedSpec_S10_IngestionFanOut` |
| S11 | `targets: ["ingestion.nightly"]` → one row for that ingestion only, other ingestions absent | ✓ `TestSaveNormalizedSpec_S11_NamedIngestionTarget` |
| S12 | Multi-target `["agent", "ingestion.hook"]` → one row per resolved role, other roles absent | ✓ `TestSaveNormalizedSpec_S12_MultiTarget` |
| S13 | optional flag stored correctly on each row | ✓ `TestSaveNormalizedSpec_S13_OptionalFlagStoredOnRow` |
| S14 | user_var_name and account_var_ref stored on row | ✓ `TestSaveNormalizedSpec_VarRefsStored` |
| S15 | Update-deploy clears all prior rows then inserts the new set | ✓ `TestSaveNormalizedSpec_BuildEnv_UpdateDeployClears` / `TestSaveNormalizedSpec_S15_UpdateDeployClears` |
| S16 | No duplicate (deployment_id, role, env_name) rows emitted | ✓ `TestSaveNormalizedSpec_S16_NoDuplicateRows` |
| S17 | RepairNormalizedSpec does not clear existing build_env rows | ✓ `TestRepairNormalizedSpec_PreservesBuildEnvRows` |

---

## 4. Variable Resolution Pipeline

Once a deployment is saved, variables flow through four sub-steps before they can be mounted into containers.

#### Reading variables back

When a deployment is re-applied (e.g. after a node restart or a manual redeploy), the stored variable values need to be retrieved and decrypted so the spec can be reconstructed with the original secrets intact.

| # | Test Case | Status |
|---|-----------|--------|
| R1 | All stored variable rows are returned for the deployment | ✓ `TestGetBuildEnv_ReturnsAllRows` / `TestGetBuildEnv_R1_AllRowsReturned` |
| R2 | Non-secret values come back as the original plaintext | ✓ `TestGetBuildEnv_R2_PlaintextStoredWithoutNonce` |
| R3 | Secret values come back encrypted; decrypting them with the deployment key recovers the original value | ✓ `TestGetBuildEnv_R3_SecretStoredAsCiphertextWithNonce` |
| R4 | A variable that targets multiple containers has the same value in every stored row | ✓ `TestGetBuildEnv_R4_MultiRoleRowsHaveIdenticalValues` |
| R5 | A deployment with no variables returns an empty list, not an error | ✓ `TestGetBuildEnv_R5_EmptyDeploymentReturnsNilNotError` |

#### Rebuilding the variable list

When the admin UI or CLI needs to show the current variable values for a deployment, the stored rows are rebuilt into the original variable shape — including which containers each variable targets and any account variable reference.

| # | Test Case | Status |
|---|-----------|--------|
| V1 | All variables for a deployment are returned | ✓ `TestGetDeploymentVariables_RoleReconstruction` |
| V1a | A variable targeting the agent container reports target = agent | ✓ `TestGetDeploymentVariables_V1_AgentRoleReconstructsTarget` |
| V2 | A variable targeting the messaging sidecar reports target = interface.slack | ✓ `TestGetDeploymentVariables_V2_MessagingRoleReconstructsTarget` |
| V3 | A variable targeting a specific ingestion job reports target = ingestion.name | ✓ `TestGetDeploymentVariables_V3_IngestionRoleReconstructsTarget` |
| V4 | A variable backed by an account variable reference reports that reference | ✓ `TestGetDeploymentVariables_RefsRoundtrip` |
| V5 | A variable targeting multiple containers reports all of them | ✓ `TestGetDeploymentVariables_V5_MultiTargetReconstructsAllTargets` |
| V6 | Secret flag is preserved when rebuilding the variable list | ✓ `TestGetDeploymentVariables_V6_SecretFlagPreserved` |
| V7 | Optional flag is preserved when rebuilding the variable list | ✓ `TestGetDeploymentVariables_V7_OptionalFlagPreserved` |

#### Deciding which variable goes to which container

Each variable in the spec declares which containers should see it. This step enforces those boundaries — a slack token should never appear in the agent container, and a database URL should never appear in the messaging sidecar.

| # | Test Case | Status |
|---|-----------|--------|
| P1 | A variable targeting only the agent is absent from the messaging and ingestion containers | ✓ `TestResolve_SlackVarsDoNotLeakToAgent` |
| P2 | A variable targeting only the messaging sidecar is absent from the agent container | ✓ `TestResolve_SlackVarsDoNotLeakToAgent` |
| P3 | Slack tokens never reach the agent container | ✓ `TestResolve_SlackVarsDoNotLeakToAgent` |
| P4 | A variable targeting all ingestion jobs appears in every ingestion container | ✓ `TestResolve_DBURLFansOutToAgentAndEachIngestion` |
| P5 | A variable targeting a specific ingestion job appears only in that job | ✓ `TestResolve_NamedIngestionTargetScopedToThatJobOnly` |
| P6 | The messaging sidecar always receives its required platform config regardless of what the user sets | ✓ `TestResolve_MessagingHardcodedKnobs` |
| P7 | The identity token is available in both the agent and the messaging sidecar | ✓ `TestResolve_MessagingHardcodedKnobs` |
| P8 | When two postgres stores share the same provider, each gets its own uniquely named credential env vars on the agent | ✓ `TestResolve_KnowledgePostgresPerStoreRenaming` |
| P9 | The same env var name never appears twice for the same container | ✓ `TestResolve_NoDuplicateRowsForSameEnvName` |
| P10 | The list of distinct container types in a resolution is computed correctly | ✓ `TestRolesIn_ReturnsDistinctRoles` |
| P11 | The collector container receives its required platform env vars (agent name, deployment ID, Langfuse config) | ✓ `TestResolve_CollectorReceivesPlatformEnvVars` |
| P12 | A knowledge container receives the credential env vars for its store | ✓ `TestResolve_KnowledgeContainerReceivesStoreEnvVars` |
| P13 | An ingestion container receives the env vars scoped to that job | ✓ `TestResolve_IngestionContainerReceivesItsEnvVars` |

#### Separating plain config from secrets

Before K8s resources are created, each container's variables are split into two buckets: plain configuration that goes into a ConfigMap (readable), and secrets that go into a K8s Secret (protected).

| # | Test Case | Status |
|---|-----------|--------|
| P14 | Non-secret variables end up in the ConfigMap for that container | ✓ `TestProject_SplitsBySecret` |
| P15 | Secret variables end up in the K8s Secret for that container | ✓ `TestProject_SplitsBySecret` |
| P16 | Variables scoped to a different container are excluded entirely | ✓ `TestProject_FiltersToOneRole` |
| P17 | A container with no variables produces empty ConfigMap and Secret | ✓ `TestProject_P17_EmptyResolutionProducesEmptyMaps` |

---

## 5. K8s Apply

Takes the resolved spec and creates or updates every K8s resource: Deployments, StatefulSets, CronJobs, Jobs, Services, Ingresses, ConfigMaps, Secrets, and NetworkPolicies. This is the final step — what ends up here is exactly what runs inside the cluster.

#### Variables and secrets on containers

| # | Test Case | Status |
|---|-----------|--------|
| K1 | Agent ConfigMap contains the agent's non-secret variables | ✓ `TestApplyDeploymentSpec_WithModel` |
| K2 | Agent Secret contains the agent's secret variables | ✓ `TestApplyDeploymentSpec_WithSecretVariables` |
| K3 | Managed API key (e.g. ANTHROPIC_API_KEY injected by the platform) lands in the agent Secret | ✓ `TestApplyDeploymentSpec_ManagedAnthropicKey` |
| K4 | Messaging sidecar ConfigMap contains vars targeted at the interface adapter | ✓ `TestApplyDeploymentSpec_InterfaceEnvInMessagingContainer` |
| K5 | Messaging sidecar Secret contains SLACK_BOT_TOKEN and SLACK_APP_TOKEN | ✓ `TestApplyDeploymentSpec_SlackSecretsOnMessagingContainer` |
| K6 | Messaging container mounts its own Secret via envFrom — not the agent Secret | ✓ `TestApplyDeploymentSpec_SlackSecretsOnMessagingContainer` |
| K7 | Agent container does not mount the messaging Secret | ✓ `TestApplyDeploymentSpec_SlackSecretsOnMessagingContainer` |
| K8 | Ingestion container mounts the deployment ConfigMap and Secret via envFrom | ✓ `TestApplyDeploymentSpec_K8_IngestionContainerMountsConfigMapAndSecret` |
| K9 | No Secret resource is created when all secret values are empty or stripped | ✓ `TestApplyDeploymentSpec_SlackSecretsOnMessagingContainer` ("stripped spec" subtest) |
| K10 | Secret variable values never appear in the ConfigMap — they route exclusively to the Secret | ✓ `TestApplyDeploymentSpec_K10_SecretValueNotInConfigMap` |
| K11 | SLACK_CONFIG env var appears in the messaging container when a slack allowlist is configured | ✓ `TestApplyDeploymentSpec_TemplateContract_SlackAllowlist` |
| K12 | Knowledge store credentials reach the agent container as secretKeyRef entries | ✓ `TestApplyDeploymentSpec_KnowledgeCredSecretKeyRefs_Agent` |
| K13 | Knowledge store credentials also reach ingestion containers as secretKeyRef entries | ✓ `TestApplyDeploymentSpec_KnowledgeCredSecretKeyRefs_Ingestion` |
| K14 | When a user sets a direct env entry and an envFrom source both expose the same key, the direct entry wins | ✓ `TestBuildContainerStatuses_DedupesEnvDirectOverridesEnvFrom` |

#### Workload resources

| # | Test Case | Status |
|---|-----------|--------|
| K15 | Persistent knowledge store is deployed as a StatefulSet | ✓ `TestApplyDeploymentSpec_WithKnowledgePersistent` |
| K16 | Non-persistent knowledge store is deployed as a Deployment | ✓ `TestApplyDeploymentSpec_WithKnowledgeNonPersistent` |
| K17 | Model is deployed as a Deployment or StatefulSet alongside the agent | ✓ `TestApplyDeploymentSpec_WithModel` |
| K18 | Integration tool is deployed as a Deployment | ✓ `TestApplyDeploymentSpec_WithTool` |
| K19 | Schedule ingestion is deployed as a CronJob | ✓ `TestApplyDeploymentSpec_WithIngestionSchedule` |
| K20 | Startup ingestion is deployed as a one-shot Job | ✓ `TestApplyDeploymentSpec_WithIngestionStartup` |
| K21 | Webhook ingestion is deployed as a Deployment with a Service | ✓ `TestApplyDeploymentSpec_WithIngestionWebhook` |
| K22 | Observability collector is deployed when enabled | ✓ `TestApplyDeploymentSpec_WithObservability` |
| K23 | Observability uses a custom image when one is specified | ✓ `TestApplyDeploymentSpec_ObservabilityCustomImage` |
| K24 | No collector resources are created when observability is disabled | ✓ `TestApplyDeploymentSpec_ObservabilityDisabled` |
| K25 | Workload names produced by the applier match what SaveNormalizedSpec would store in the DB | ✓ `TestApplyDeploymentSpec_WorkloadNamesMatchNormalized` |
| K26 | Full stack (agent + model + knowledge + tool + ingestion + observability) deploys without errors | ✓ `TestApplyDeploymentSpec_FullStack` |

#### Interfaces, adapters, and ingress

| # | Test Case | Status |
|---|-----------|--------|
| K27 | Messaging sidecar is deployed as a container in the agent pod, not as a separate Deployment | ✓ `TestApplyDeploymentSpec_WithSlackInterface` |
| K28 | Web adapter exposes the messaging HTTP endpoint to external traffic | ✓ `TestApplyDeploymentSpec_WithWebInterfaceExpose` |
| K29 | Frontend agent exposes its own HTTP endpoint to external traffic | ✓ `TestApplyDeploymentSpec_WithFrontendExpose` |
| K30 | HTTP endpoint is only exposed when an adapter that requires it is selected | ✓ `TestApplyDeploymentSpec_AdapterExposedWhenDefined` |

#### Identity and auth

| # | Test Case | Status |
|---|-----------|--------|
| K31 | ASTRO_AUTHZ_TOKEN has the same value on both the agent and messaging containers | ✓ `TestApplyDeploymentSpec_IdentityTokenInjectedIntoAgentAndMessaging` |
| K32 | No token is injected when the deploy token secret is not configured | ✓ `TestApplyDeploymentSpec_IdentityTokenSkippedWhenSecretUnset` |
| K33 | Grants are rejected when no deploy token secret is set | ✓ `TestApplyDeploymentSpec_RefusesGrantsWithoutSecret` |
| K34 | OIDC auth secret is created and annotated when `auth.web.type=oidc` | ✓ `TestApplyDeploymentSpec_OIDCAuth_EnabledWhenOptIn` |
| K35 | OIDC auth is not applied when the auth type is not oidc | ✓ `TestApplyDeploymentSpec_OIDCAuth_DisabledWhenNotOptIn` |
| K36 | OIDC auth is not applied when the server is not configured for OIDC | ✓ `TestApplyDeploymentSpec_OIDCAuth_DisabledWhenServerNotConfigured` |
| K37 | Signed token's `anyone_adapters` claim contains "slack" when an anyone grant is configured | ✓ `TestApplyDeploymentSpec_SlackAnyoneGrantInToken` |
