# K8s Builder Test Coverage

Test coverage for all Kubernetes manifest builders in `apps/astro-server/internal/k8s/`.

## Provider Registry Coverage

Every provider in `packages/astro-spec/provider.go` is tested for correct manifest generation.

### Self-Hosted Providers

| Provider | Section | Port | Extra Ports | Mount Path | Health Check | StatefulSet | Deployment HC |
|----------|---------|------|-------------|------------|--------------|-------------|---------------|
| qdrant | knowledge | 6333 | gRPC 6334 | /qdrant/storage | HTTP /healthz | Tested | Tested |
| redis | knowledge | 6379 | — | /data | Exec redis-cli ping | Tested | Tested |
| postgres | knowledge | 5432 | — | /var/lib/postgresql/data | Exec pg_isready -U postgres | Tested | Tested |
| neo4j | knowledge | 7474 | Bolt 7687 | /data | HTTP / | Tested | Tested |

### Cloud Providers

| Provider | Section | Credential Key | Tested |
|----------|---------|----------------|--------|
| anthropic | models | ANTHROPIC_API_KEY | Yes |
| openai | models | OPENAI_API_KEY | Yes |
| google | models | GOOGLE_API_KEY | Yes |
| gemini | models | GEMINI_API_KEY | Yes |
| cohere | models | COHERE_API_KEY | Yes |
| pinecone | knowledge | PINECONE_API_KEY | Yes |
| github | tools | GITHUB_TOKEN | Yes |
| gitlab | tools | GITLAB_TOKEN | Yes |

## Builder Coverage

### StatefulSet (`statefulset.go`)

Test file: `statefulset_test.go`

| Test | What it verifies |
|------|-----------------|
| Table-driven | Kind, name, replicas, serviceName for all providers |
| qdrant ports and mount | Port 6333, gRPC 6334, mount /qdrant/storage, PVC 10Gi |
| redis port and mount | Port 6379, mount /data |
| postgres port and mount | Port 5432, mount /var/lib/postgresql/data |
| neo4j port mount and extra ports | Port 7474, bolt 7687, mount /data |
| healthcheck qdrant | HTTPGet /healthz |
| healthcheck redis | Exec redis-cli ping |
| ConfigMap and Secret envFrom | Both refs present |
| Error: missing port | Returns error when no port and no provider default |
| Error: missing mount path | Returns error when provider has no mount path |

### Deployment (`deployment.go`)

Test file: `deployment_test.go`

| Test | What it verifies |
|------|-----------------|
| minimal defaults | Port 8080, CPU 50m, memory 128Mi, no probes, PullAlways |
| GPU enabled | nvidia.com/gpu:1, CPU 2, memory 16Gi, node selector |
| ConfigMap and Secret refs | envFrom entries |
| container env vars | Environment map injected |
| healthcheck exec | Custom test command, initial delay 10s |
| healthcheck HTTP path | HTTPGet probe with path |
| healthcheck redis | Exec redis-cli ping |
| healthcheck qdrant | HTTPGet /healthz:6333 |
| healthcheck postgres | Exec pg_isready |
| healthcheck neo4j | HTTPGet /:7474 |
| custom timing | PeriodSeconds, TimeoutSeconds, FailureThreshold |
| custom port | ContainerPort override |

### MessagingDeployment (`deployment.go`)

Test file: `deployment_test.go`

| Test | What it verifies |
|------|-----------------|
| slack interface | Port 9090, SLACK_ENABLED/GRPC_ENABLED/SLACK_SOCKET_MODE env, secret envFrom |
| web enabled | 2 ports (grpc + http:8080), WEB_ENABLED=true |
| without secret | No envFrom entries |

### CollectorDeployment (`deployment.go`)

Test file: `deployment_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | OTLP ports 4317/4318, ConfigMap envFrom, Galileo env, resources 25m/128Mi |
| without optional fields | No envFrom, only ASTRO env vars |
| custom ImagePullPolicy | IfNotPresent |
| ASTRO identity env vars | ASTRO_AGENT_NAME, ASTRO_AGENT_VERSION, ASTRO_DEPLOYMENT_ID |
| GALILEO_LOG_STREAM injected | Present when set |
| GALILEO_LOG_STREAM omitted | Absent when empty |
| full config all fields | All Galileo + ASTRO vars together |

### CronJob (`cronjob.go`)

Test file: `cronjob_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | Schedule, ForbidConcurrent, history limits (3/1), OnFailure, env vars, envFrom |
| no secret or configmap | 0 envFrom entries |
| image and resources | Image propagated, PullAlways, CPU 50m request, memory 256Mi limit |

### Job (`job.go`)

Test file: `cronjob_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | Kind, name, BackoffLimit=3, OnFailure, container name |
| TTL and env | TTL 86400s, labels, env vars from ingestion, envFrom count |
| no secret or configmap | 0 envFrom entries |

### IngestionDeployment (`job.go`)

Test file: `cronjob_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | Kind, replicas=1, RestartAlways, port 8080, custom port 9090 |
| env and labels | Agent label, selector, PullIfNotPresent override, env vars, envFrom, CPU 50m |

### Service (`service.go`)

Test file: `service_test.go`

| Test | What it verifies |
|------|-----------------|
| default ClusterIP | Type ClusterIP, port 8080 |
| custom port and LoadBalancer | Type, port, targetPort |
| port name and protocol | Name "http", protocol TCP |
| labels and selectors | astro.dev/agent, app.kubernetes.io/component |

### Ingress (`ingress.go`)

Test file: `ingress_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | ALB annotations (scheme, target-type, listen-ports, ssl-redirect, external-dns), cert ARN, group name, host rule, path, backend |
| minimal config | Optional annotations absent when not set |
| GenerateIngressHost | Determinism, uniqueness, 59-char limit, DNS format, hash format |
| GenerateIngestionIngressHost | Budget allocation, determinism, uniqueness |
| truncateLabel | Within/at/over limit, trailing hyphen trim |

### ConfigMap (`configmap.go`)

Test file: `configmap_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | Name format, namespace, Kind, data preservation, entry count, agent label |
| empty data | Kind correct, 0 entries |

### Secret (`secret.go`)

Test file: `secret_test.go`

| Test | What it verifies |
|------|-----------------|
| full config | Name format, namespace, Opaque type, Kind, key uppercasing, value bytes, agent label |
| empty values | Opaque type, 0 entries |
| mixed case keys | ALREADY_UPPER, LOWER_CASE, MIXED all present |

## Spec-Level Coverage

### Credential Key Generation (`packages/astro-spec/envresolver_test.go`)

- All cloud providers: anthropic, openai, google, gemini, cohere, pinecone, github, gitlab
- Custom provider (Jira): suffix-only names, {UPPER(provider)}_{varName} key construction
- Duplicate-entry handling: bare key, qualified keys, name-matches-provider
- Cross-section custom providers
- Description and Optional carried through

### Template Generation (`apps/astro-server/internal/deployment/template_test.go`)

- Jira integration: variables created with secret=true, ${variables.*} refs in agent env, descriptions preserved
- Custom provider: suffix-only variable names produce correct keys

### Deployment Spec Resolution (`apps/astro-server/internal/deployment/spec_resolver_test.go`)

- Jira secrets: ${variables.*} refs resolve to values, all in SecretData
- Model, knowledge, tool references
- Source references, composite references, plain values
- Platform vars, OTEL endpoint, interfaces GRPC addr
