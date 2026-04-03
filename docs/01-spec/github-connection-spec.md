# GitHub Connection Spec

**Version:** 2.0
**Date:** 2026-04-03
**Status:** Implemented

## Abstract

This spec defines a Cloudflare-style GitHub connection system for Astro. Users link a GitHub repository to an agent; every push to the configured branch triggers a server-side build and registration without running `astro push`. WorkOS Pipes provides the GitHub OAuth token. Builds run as Kubernetes Jobs using BuildKit (rootless, daemonless).

---

## 1. Motivation

The current CLI-based push flow (`astro build → astro push → astro deploy`) requires Docker, ECR credentials, and a local build environment. GitHub-connected builds move the build step to Astro infrastructure, making it possible to author and publish agents with only a code editor and a GitHub account.

---

## 2. User Experience

1. User clicks **Connect GitHub repo** on the blueprint detail page (owner only).
2. If not yet connected, WorkOS Pipes initiates GitHub OAuth (`repo` + `admin:repo_hook` scopes).
3. After OAuth completes, a repo selector dialog appears. User picks a repo and branch.
4. Astro installs a webhook on the repository.
5. On the next push to the connected branch:
   - A build job starts automatically; status updates in the panel (`pending → building → registered | failed`).
   - The panel polls every 5 seconds while a build is in-flight; stops when the latest build reaches a terminal state.
6. Once `registered`, the new build appears in the deploy flow identically to a CLI-pushed build.
7. **Rebuild** button (↻) triggers a fresh build from the current HEAD without a push.
8. **Logs** button (📋) on each build row opens a dialog with pod logs per container.
9. User can disconnect at any time; the webhook is removed.

---

## 3. Architecture

```
GitHub push event
       │
       ▼
POST /webhooks/github              ← HMAC-SHA256 verified, no session auth
       │
       ├─ Look up connection by repo full name
       ├─ Verify push is to connected branch
       ├─ Create github_builds record (status: pending)
       └─ Enqueue GitHubBuildWorker (River, queue: github_build)
                │
                ▼
        GitHubBuildWorker.Work()   ← 25-min timeout, MaxAttempts: 1
                │
                ├─ status → building
                ├─ Get GitHub token via WorkOS Pipes API
                ├─ Fetch astropods.yml from GitHub contents API at exact SHA
                ├─ Create ephemeral K8s Secret with GitHub token
                │
                ├─ Create K8s Job in as0-builds namespace:
                │     init: git-clone   → clones repo into /workspace (HOME=/tmp, safe.directory set)
                │     init: ecr-login   → writes docker config from ECR via IRSA (prod only)
                │     main: buildkit    → reads /workspace, builds Dockerfile
                │                         moby/buildkit:v0.21.0-rootless, --no-push in local dev
                │
                ├─ Poll Job status every 15s (max 20 min)
                │     on success: fetch AGENT.md, register in AgentIndex
                │     on failure: fetch pod logs → stored in error field
                │
                └─ status → registered | failed
                      (DB updates use context.Background(), not job ctx)
```

### 3.1 WorkOS Pipes

The server calls the WorkOS Pipes REST API directly (not in the SDK):
- `POST /data-integrations/github/authorize` — get OAuth redirect URL
- `POST /data-integrations/github/token` — get/refresh access token

Tokens are fetched on demand (WorkOS handles refresh); they are NOT stored in the database.

### 3.2 Webhook Delivery

Webhook payload URL: `{GITHUB_CALLBACK_URL}/webhooks/github` (falls back to `FRONTEND_URL`).  
Webhook secret is a random 32-byte hex string per connection, verified with `HMAC-SHA256`.  
Only `push` events to the configured branch trigger builds; all others return `200 OK`.

### 3.3 Server-Side Build (BuildKit)

BuildKit runs rootless daemonless (`moby/buildkit:v0.21.0-rootless`) in a K8s Job:

**Local dev** (`K8S_CLIENT_MODE=local`): single BuildKit container, `--no-push`. Builds the image to verify the Dockerfile but does not push to any registry.

**Production**: init container fetches ECR login credentials via IRSA → docker config on shared volume. BuildKit pushes to ECR with `--output type=image,name={dest},push=true`.

In both modes: init container clones the repo from GitHub using an ephemeral K8s Secret (token not visible in Job args). Namespace (`as0-builds`) and service account (`build-worker`) are auto-created if missing.

---

## 4. Data Model

### `github_connections`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `account_id` | UUID | FK → accounts |
| `agent_name` | string | Agent this connection is for |
| `workos_user_id` | string | WorkOS user ID (for Pipes token lookup) |
| `repo_full_name` | string | e.g. `octocat/hello-world` |
| `branch` | string | e.g. `main` |
| `webhook_id` | int64 | GitHub webhook ID (for removal) |
| `webhook_secret` | string | HMAC secret for payload verification |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

One connection per agent. Relinking removes the old webhook before installing the new one.

### `github_builds`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key (`build_record_id` in worker args) |
| `connection_id` | UUID | FK → github_connections |
| `account_id` | UUID | Denormalized for queries |
| `agent_name` | string | |
| `build_id` | string | 8-char hex, matches AgentIndex build ID |
| `commit_sha` | string | Full SHA of the triggering push |
| `branch` | string | |
| `status` | enum | `pending \| building \| registered \| failed` |
| `error` | text | Failure message + last 100 lines of pod logs |
| `enqueued_at` | timestamp | |
| `completed_at` | timestamp | |

---

## 5. API Surface

All routes are under `/api/v1/agents/:account/:name/`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `.../github/connect` | session | Start WorkOS Pipes OAuth; returns `{redirect_url}` or `{connected: true}` |
| `GET` | `.../github/callback` | session | WorkOS return_to; stores token state, redirects to frontend |
| `GET` | `.../github/repos` | session | List user's GitHub repos via Pipes token |
| `POST` | `.../github/link` | session | Install webhook and save connection |
| `DELETE` | `.../github` | session | Remove webhook and connection |
| `GET` | `.../github` | session | Connection status + last 10 builds |
| `POST` | `.../github/rebuild` | session | Trigger build from current branch HEAD |
| `GET` | `.../github/builds/:build_id/logs` | session | Stream pod logs for a build job |
| `POST` | `/webhooks/github` | HMAC | Receive GitHub push events |

---

## 6. River Worker

```
GitHubBuildArgs {
    ConnectionID  string  // uuid
    CommitSHA     string
    BuildID       string  // 8-char hex
    BuildRecordID string  // uuid, matches github_builds.id
}
```

- `MaxAttempts: 1` — failures surface as build records, not River retries
- `Timeout: 25 minutes` — explicit override on `GitHubBuildWorker.Timeout()`
- Unique by all args (each build has a unique `BuildRecordID`, so no cross-build deduplication)
- All `github_builds` status updates use `context.Background()` to survive job context cancellation

---

## 7. BuildKit Job Spec

Job name: `build-{build-id}-agent` in `as0-builds` namespace.  
Service account: `build-worker` (auto-created; needs IRSA ECR push in production).  
Security context: rootless (`seccompProfile: Unconfined`, `runAsUser: 1000`).  
`BUILDKITD_FLAGS=--oci-worker-no-process-sandbox` required for rootless K8s.  
Resources: `1-2 CPU / 2-4Gi memory`. TTL: 1 hour after completion.

---

## 8. Security

- **HMAC-SHA256** verification on every webhook payload before processing.
- **GitHub token** fetched on-demand from WorkOS Pipes; never stored in the database.
- **Ephemeral K8s Secret** per build for git clone credentials; deleted after job creation.
- **ECR credentials** injected via IRSA (pod identity) — no long-lived credentials in Job spec.
- **Build isolation**: jobs run in dedicated `as0-builds` namespace, separate from agent deployment namespaces.

---

## 9. Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_CALLBACK_URL` | `FRONTEND_URL` | Public base URL for OAuth callbacks and webhook delivery |
| `GITHUB_BUILD_NAMESPACE` | `as0-builds` | K8s namespace for build jobs |
| `GITHUB_BUILD_SERVICE_ACCOUNT` | `build-worker` | K8s service account (needs IRSA ECR push in prod) |

---

## 10. Migration

No changes to `astropods.yml` format or the deploy flow. GitHub-connected builds produce identical `AgentIndex` entries to CLI-pushed builds. Existing CLI users are unaffected; the connection is opt-in per agent.
