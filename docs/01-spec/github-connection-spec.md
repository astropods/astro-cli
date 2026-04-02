# GitHub Connection Spec

**Version:** 1.0
**Date:** 2026-04-02
**Status:** Draft

## Abstract

This spec defines a Cloudflare-style GitHub connection system for Astro. Users link a GitHub repository to an agent; every push to `main` triggers a server-side build and registration without the user running `astro push`. WorkOS Pipes provides the GitHub OAuth token. Builds run as Kubernetes Jobs using Kaniko, mirroring exactly what the CLI does today.

---

## 1. Motivation

The current CLI-based push flow (`astro build → astro push → astro deploy`) requires the user to have Docker installed, ECR credentials, and a local build environment. This is a barrier to adoption. GitHub-connected builds move the build step to Astro infrastructure, making it possible to author and publish agents with only a code editor and a GitHub account.

---

## 2. User Experience

1. From the agent detail page, the user clicks **Connect GitHub repo**.
2. If no GitHub connection exists on the account, WorkOS Pipes initiates OAuth to grant repository access.
3. The user selects a repository and branch (default: `main`).
4. Astro installs a webhook on the repository.
5. On the next push to the connected branch:
   - A build job starts automatically.
   - The build status surfaces on the agent's Builds tab (pending → building → registered or failed).
6. Once registered, the new build appears in the deploy flow exactly like a CLI-pushed build.
7. The user can disconnect the repo at any time; the webhook is removed and future pushes are ignored.

---

## 3. Architecture

```
GitHub push event
       │
       ▼
POST /webhooks/github          ← new HTTP handler (no auth, HMAC verified)
       │
       ▼
GitHubBuildWorker (River)      ← enqueued with repo clone URL + commit SHA
       │
       ├─ Clone repo (GitHub token from connection record)
       ├─ Parse astropods.yml
       ├─ Launch Kaniko Job per container in spec
       │     └─ Kaniko pushes image to ECR namespace
       └─ Register spec in AgentIndex (same path as CLI push handler)
              │
              ▼
       Build visible in UI, ready for deploy
```

### 3.1 WorkOS Pipes

WorkOS Pipes delivers a GitHub OAuth token scoped to the user's installation. The server exchanges the Pipes connection for a token with `repo` and `admin:repo_hook` scopes. This token is stored encrypted (KMS, same mechanism as other secrets) in the `github_connections` table and refreshed on use.

When the user initiates a connection, the server:
1. Calls WorkOS Pipes to get an authorization URL for GitHub.
2. Redirects the user; WorkOS completes OAuth and calls back with a connection ID.
3. Server fetches the access token from WorkOS using the connection ID.
4. Stores the token and creates the webhook.

### 3.2 Webhook Delivery

Astro installs a GitHub webhook on the selected repository pointing at `https://api.astropods.ai/webhooks/github`. The webhook secret is a random 32-byte hex string stored alongside the connection record. All incoming payloads are verified with `HMAC-SHA256` before processing.

Only `push` events to the configured branch are acted on; all others return `200 OK` immediately.

### 3.3 Server-Side Build (Kaniko)

The `GitHubBuildWorker` River worker:
1. Clones the repo at the push commit SHA using the stored GitHub token.
2. Parses `astropods.yml` from the repo root.
3. For each container in the spec's `agent.build` (and any ingestion containers), launches a Kubernetes `Job` using the [Kaniko](https://github.com/GoogleContainerTools/kaniko) executor image.
4. Kaniko receives the build context via a temporary S3 presigned URL (tarball of the cloned repo) and pushes the built image directly to the account's ECR namespace, tagged with the build ID.
5. The worker waits for all Jobs to complete (polling, max 20 minutes), then calls the agent registration logic (same as `POST /agents/:name` from the CLI).
6. On failure, the build record is marked `failed` with the Kaniko Job log excerpt.

Build ID generation is identical to the CLI: random 8-char hex. The ECR namespace is the account's existing namespace.

---

## 4. Data Model

### `github_connections`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `account_id` | UUID | FK → accounts |
| `agent_name` | string | Agent this connection is for |
| `workos_connection_id` | string | WorkOS Pipes connection ID |
| `encrypted_token` | bytes | KMS-encrypted GitHub access token |
| `repo_full_name` | string | e.g. `octocat/hello-world` |
| `branch` | string | e.g. `main` |
| `webhook_id` | int64 | GitHub webhook ID (for removal) |
| `webhook_secret` | string | HMAC secret for payload verification |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

One connection per agent. Replacing the connection (user selects a different repo) removes the old webhook first.

### `github_builds`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `connection_id` | UUID | FK → github_connections |
| `account_id` | UUID | Denormalized for queries |
| `agent_name` | string | |
| `build_id` | string | 8-char hex, matches AgentIndex build ID |
| `commit_sha` | string | Full SHA of the triggering push |
| `branch` | string | |
| `status` | enum | `pending \| building \| registered \| failed` |
| `error` | text | Failure message if failed |
| `enqueued_at` | timestamp | |
| `completed_at` | timestamp | |

---

## 5. API Surface

### GitHub OAuth

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/accounts/:account/github/connect` | Starts WorkOS Pipes OAuth; returns redirect URL |
| `GET` | `/accounts/:account/github/callback` | WorkOS callback; stores token, creates webhook |
| `DELETE` | `/accounts/:account/agents/:name/github` | Removes webhook and connection |

### Connection Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/accounts/:account/agents/:name/github` | Returns connection record (no token) and last 10 builds |

### Webhook Receiver

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/webhooks/github` | Receives push events from GitHub; unauthenticated, HMAC verified |

The webhook handler looks up the connection by `X-GitHub-Hook-ID` or repository full name in the payload, verifies the signature, and enqueues a `GitHubBuildWorker` job. It responds `202 Accepted` before the job completes.

---

## 6. River Worker

```
GitHubBuildArgs {
    ConnectionID  uuid
    CommitSHA     string
    BuildID       string
    BuildRecordID uuid
}
```

The worker runs with:
- `MaxAttempts: 1` (build failures are surfaced as build records, not retried automatically)
- `Timeout: 25 minutes`
- Unique by `ConnectionID + CommitSHA` (deduplicates rapid pushes)

Rapid pushes within the uniqueness window are deduplicated; only the first enqueued job runs. Subsequent pushes while a build is in-flight are queued normally since the running job will have consumed the unique slot.

---

## 7. Kaniko Job Spec

Each Kaniko Job is named `build-{build-id}-{component}` in the `astro-builds` namespace (dedicated, separate from deployment namespaces). The Job mounts no persistent volumes; the build context is fetched from S3 by the Kaniko init container.

Resource limits: `2 CPU / 4Gi memory` (configurable via server config). Jobs are garbage-collected after 1 hour regardless of outcome.

---

## 8. Security

- **HMAC verification** on every webhook payload before any processing.
- **GitHub token** stored KMS-encrypted; decrypted in-memory only during clone and webhook operations.
- **Repo access** is scoped to the specific repository via WorkOS Pipes (no org-wide token).
- **Build isolation**: each Kaniko Job runs in `astro-builds` namespace with a dedicated service account that has `push` access only to the account's ECR prefix (`ecr:{account-id}/*`).
- **No secrets in build context**: the S3 tarball contains only the repo contents; AWS credentials are injected via IRSA, not environment variables visible to Kaniko.

---

## 9. Build Status in the UI

The agent detail page's **Builds** tab already lists builds from `AgentIndex`. GitHub builds appear here once registered. During the build phase (before registration), status is read from `github_builds` and surfaced as a "Building…" entry with a link to the triggering commit. Build logs (Kaniko Job stdout) are streamed via a new `GET /accounts/:account/agents/:name/github/builds/:build-id/logs` endpoint that proxies Kubernetes pod logs.

---

## 10. Migration

No changes to `astropods.yml` format or the deploy flow. GitHub-connected builds produce identical `AgentIndex` entries to CLI-pushed builds. Existing CLI users are unaffected; the connection is opt-in per agent.
