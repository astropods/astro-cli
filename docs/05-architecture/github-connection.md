# GitHub Connection Architecture

Automated builds from GitHub: when a user pushes to a linked repo, Astro
fetches the code, builds a container image with BuildKit inside Kubernetes,
pushes it to ECR, and registers the new agent version — all without the CLI.

---

## Overview

```
User pushes to GitHub
        │
        ▼
POST /webhooks/github  ──── HMAC verify ──── River queue
                                                   │
                                                   ▼
                                         GitHubBuildWorker
                                         ├─ fetch astropods.yml
                                         ├─ K8s BuildKit Job
                                         │   ├─ git clone (init)
                                         │   ├─ ecr login (init)
                                         │   └─ buildkit (main)
                                         └─ agentIndex.Register()
```

---

## OAuth Flow (WorkOS Pipes)

GitHub OAuth is delegated entirely to **WorkOS Pipes** — Astro never handles
OAuth codes or stores GitHub tokens directly.

```
Frontend                  API Server             WorkOS Pipes
   │                          │                       │
   │  POST /github/connect    │                       │
   │─────────────────────────▶│                       │
   │                          │  GetAccessToken()     │
   │                          │──────────────────────▶│
   │                          │  ErrNotInstalled      │
   │                          │◀──────────────────────│
   │                          │  GetAuthorizationURL()│
   │                          │──────────────────────▶│
   │  { redirect_url }        │◀── redirect URL ──────│
   │◀─────────────────────────│                       │
   │                          │                       │
   │  (user authenticates with GitHub via Pipes)      │
   │                          │                       │
   │  GET /github/callback    │                       │
   │─────────────────────────▶│                       │
   │                          │  GetAccessToken()     │
   │                          │──────────────────────▶│
   │                          │◀── token ─────────────│
   │  { connected: true }     │                       │
   │◀─────────────────────────│                       │
```

`pipes.Client` (`internal/pipes/client.go`) wraps the WorkOS Pipes REST API.
Tokens are stored in WorkOS and retrieved on demand by user ID — Astro never
persists GitHub tokens.

---

## Linking a Repo to an Agent

`POST /api/v1/agents/:account/:name/github/link`

1. Calls GitHub API to install a push webhook on the repo (HMAC secret generated
   per-connection with `crypto/rand`).
2. Upserts a `github_connections` row — one row per `(account_id, agent_name)`.
   Re-linking replaces the existing connection and rotates the webhook.
3. Stores `workos_user_id` from the session — needed by the background build
   worker to retrieve a GitHub token (see [Build Worker](#build-worker) below).

`DELETE /api/v1/agents/:account/:name/github` removes the webhook from GitHub
and deletes the connection row.

---

## Data Model

### `github_connections`

One row per agent. Holds everything needed to verify webhooks and trigger builds.

| Column           | Purpose                                                                           |
| ---------------- | --------------------------------------------------------------------------------- |
| `account_id`     | Owner account (UUID)                                                              |
| `agent_name`     | Linked agent name                                                                 |
| `repo_full_name` | e.g. `owner/repo` — used to match incoming webhooks                               |
| `branch`         | Which branch triggers builds                                                      |
| `webhook_id`     | GitHub webhook ID (for deletion)                                                  |
| `webhook_secret` | HMAC key for verifying webhook payloads                                           |
| `workos_user_id` | WorkOS user who linked the repo — their OAuth grant is used for all future builds |
| `account_name`   | Display only — not used for ECR paths                                             |

Unique constraint: `(account_id, agent_name)`.

### `github_builds`

One row per build attempt (webhook push or manual rebuild).

| Column                             | Purpose                                                             |
| ---------------------------------- | ------------------------------------------------------------------- |
| `build_id`                         | Random 8-char hex — used as image tag                               |
| `commit_sha`                       | Exact commit being built                                            |
| `status`                           | `pending` → `building` → `registered` / `failed`                    |
| `step`                             | Fine-grained progress: `fetching-spec` → `building` → `registering` |
| `error`                            | Last error message (concise, from build logs)                       |
| `commit_message` / `commit_author` | Stored at enqueue time from webhook payload                         |

---

## Webhook Handler

`POST /webhooks/github` — unauthenticated, HMAC-verified.

```
1. Read body (limit 5 MB)
2. Unmarshal to find repo_full_name
3. Look up connection by repo_full_name
   └─ Not found → 200 OK (no connection for this repo, ignore)
   └─ DB error  → 500 (GitHub will retry)
4. Verify X-Hub-Signature-256 against connection.webhook_secret
5. Filter: only act on pushes to connection.branch
6. Filter: ignore branch deletions (After = all zeros)
7. Create github_builds record (status: pending)
8. Enqueue GitHubBuildArgs to River queue
9. Return 202 Accepted
```

HMAC verification (`verifyGitHubSignature`) uses `crypto/hmac` + SHA-256.
The signature check happens after the DB lookup because the per-connection
secret must be fetched first.

---

## Build Worker

`GitHubBuildWorker` processes `GitHubBuildArgs` jobs from the `github_build`
River queue. Up to 3 builds run in parallel.

**Timeout:** 25 minutes per job (River default is much shorter).
**Retries:** Max 3 attempts. Permanent failures (missing spec, bad Dockerfile)
call `river.JobCancel` to skip retries immediately.

### Work() flow

```
1. Load connection from DB
2. Get GitHub OAuth token from WorkOS Pipes (using stored workos_user_id)
3. Update step: "fetching-spec"
4. Download astropods.yml at exact commit SHA (GitHub contents API)
   └─ Not found → cancel (permanent, no point retrying)
5. Extract build.context and build.dockerfile from spec
6. Compute ECR destination: {registry}/{env}-tenant-{accountID}/{agentName}:{buildID}
7. Update step: "building"
8. runBuildKitJob() — create K8s Job and wait for completion
9. Download AGENT.md (for agent card, best-effort)
10. Update step: "registering"
11. agentIndex.Register() — writes agent version with ECR image URI
12. Update status: "registered"
```

### K8s Build Job

`runBuildKitJob()` creates a `batch/v1.Job` in the build namespace with three containers:

**Init 1 — `git-clone` (`alpine/git:2.47.2`)**
Shallow-clones the repo at the exact commit SHA into `/workspace`. The GitHub
token is mounted from an ephemeral K8s Secret (not embedded in command args).

**Init 2 — `ecr-login` (`amazon/aws-cli:2.24.21`)** *(production only)*
Gets an ECR login password via IRSA and writes `~/.docker/config.json` to a
shared `docker-config` volume for BuildKit to use when pushing.

**Main — `buildkit` (`moby/buildkit:v0.21.0-rootless`)**
Runs `buildctl-daemonless.sh` to build the image from `/workspace` and push to
ECR. When `destination` is empty (local dev), the push is skipped.

```
Volumes:
  workspace     EmptyDir  — shared between git-clone and buildkit
  buildkitd     EmptyDir  — buildkit daemon cache
  token         Secret    — GitHub token for git clone
  docker-config EmptyDir  — ECR credentials (prod only)
```

Resource requests: 1 CPU / 2 GiB. Limits: 2 CPU / 4 GiB.

**Polling:** The worker polls the Job every 15 seconds (max 20 minutes).
On failure it fetches logs from all containers, extracts the last meaningful
line, and stores it as the build error.

**Cleanup:**
- On any non-success path: Job deleted immediately (background propagation)
  to free compute.
- On success: Job TTL of 3600 seconds handles cleanup.
- Token Secret: bound to the Job via `OwnerReference` — auto-deleted when
  the Job is removed.

### ECR Image Path

```
{registry_url}/{environment}-tenant-{account_uuid}/{agent_name}:{build_id}

e.g. 123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-01kgg.../my-agent:a1b2c3d4
```

The account UUID (not name) is used — matching how `astro-registry` creates ECR
repos on CLI pushes. See [ECR Namespace doc](../changelog/ecr-tenant-correction-2026-04-06.md).

---

## Manual Rebuild

`POST /api/v1/agents/:account/:name/github/rebuild`

Same as a webhook-triggered build, except:
- Requires a valid session (user-facing, not HMAC-verified).
- Calls `gh.GetBranchHead()` to resolve the current commit SHA on the linked branch.
- Calls `gh.GetCommit()` for commit message / author metadata (best-effort, logged on failure).
- Enqueues the same `GitHubBuildArgs` job type.

---

## Build Logs

`GET /api/v1/agents/:account/:name/github/builds/:build_id/logs`

1. Verifies the build belongs to the requesting account/agent (linear scan of
   last 50 builds — sufficient at current scale).
2. Finds the K8s Pod by `job-name=build-{buildID}-agent` label selector.
3. Fetches last 500 lines from each container (init containers first, then main).
4. Returns combined logs + pod phase.

Logs are only available while the K8s Job/Pod still exists (up to 1 hour TTL
on success; deleted immediately on failure).

---

## Infrastructure Requirements

| Requirement               | Detail                                                                               |
| ------------------------- | ------------------------------------------------------------------------------------ |
| WorkOS API key            | `WORKOS_API_KEY` — for Pipes OAuth                                                   |
| K8s namespace             | `as0-builds` — created at startup if absent                                          |
| K8s ServiceAccount        | `build-sa` with IRSA role for ECR push                                               |
| ECR permissions           | `ecr:GetAuthorizationToken`, `ecr:BatchCheckLayerAvailability`, `ecr:PutImage`, etc. |
| `GITHUB_WEBHOOK_BASE_URL` | Public URL for webhook callback (uses `FRONTEND_URL` as default)                     |
| `AWS_REGION`              | For ECR login command in the build job                                               |

---

## Dependency on `workos_user_id`

`github_connections.workos_user_id` stores the WorkOS user ID of whoever
linked the repo. The build worker uses it to retrieve a GitHub OAuth token
from Pipes for every build — including webhook-triggered builds that run
long after the user's session has ended.

Implication: if that user revokes their GitHub grant or is removed from the
WorkOS org, all future builds for that connection will fail at step 2
("get github token"). Re-linking the repo with an active user fixes it.

---

## Configuration Reference

| Variable                       | Default        | Purpose                             |
| ------------------------------ | -------------- | ----------------------------------- |
| `WORKOS_API_KEY`               | —              | WorkOS Pipes OAuth                  |
| `GITHUB_BUILD_NAMESPACE`       | `as0-builds`   | K8s namespace for build jobs        |
| `GITHUB_BUILD_SERVICE_ACCOUNT` | `build-sa`     | K8s SA with ECR IRSA                |
| `GITHUB_WEBHOOK_BASE_URL`      | `FRONTEND_URL` | Webhook target registered on GitHub |
| `DEPLOYMENT_REGISTRY_URL`      | —              | ECR registry host                   |
| `DEPLOYMENT_ENVIRONMENT`       | —              | Tenant prefix (e.g. `prod`)         |
| `AWS_REGION`                   | —              | For ECR login in build jobs         |
