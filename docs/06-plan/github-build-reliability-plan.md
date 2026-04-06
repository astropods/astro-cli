# GitHub Build Job — Reliability & Observability Plan

Work items in priority order. Tackle one at a time on branch `github-connection-spec`.

---

## Item 1 — Build step tracking (progress visibility)

**Status:** [x] done

Add a `step` column to `github_builds` so the UI can show where in the pipeline a build is, not just that it's "building".

- Migration: add `step TEXT NOT NULL DEFAULT ''` to `github_builds`
- Add `UpdateBuildStep(ctx, id, step string) error` to `githubconnection.Store`
- Call `UpdateBuildStep` at each phase in `GitHubBuildWorker.Work`:
  - `"fetching-spec"` — before `fetchAstroSpec`
  - `"cloning"` — before `runBuildKitJob` (K8s job created, git-clone init container running)
  - `"building"` — buildkit container running (derived from pod init container completion)
  - `"registering"` — after build job succeeds, before `agentIndex.Register`
- Expose `step` in the `Build` struct and the `/github/status` API response

---

## Item 2 — Store commit metadata at enqueue time

**Status:** [x] done

`github_builds` only stores `commit_sha` and `branch`. The webhook payload has `commit_message` and `commit_author` already; storing them avoids a GitHub API call when rendering build history.

- Migration: add `commit_message TEXT NOT NULL DEFAULT ''` and `commit_author TEXT NOT NULL DEFAULT ''`
- Populate both fields in `Store.CreateBuild` from the webhook push event
- Expose in `Build` struct and API response

---

## Item 3 — Input validation before K8s job creation

**Status:** [x] done

Two silent failure modes exist before the expensive K8s job starts:

1. Missing `astropods.yml`: `fetchFileContent` returns `"", nil` on 404 → `yaml.Unmarshal("")` succeeds with a zero-value spec → build proceeds with an empty agent name.
2. Empty agent name after derivation: `agentIndex.Register` is called with `""` if the spec name stripping produces an empty string.

Changes:
- After `fetchAstroSpec`, check that the returned `specYAML` is non-empty; if empty, fail immediately with a clear message ("astropods.yml not found in repo at this commit")
- After agent name derivation (lines 111–115), validate `agentName != ""`; fail fast if empty
- Both failures use `w.fail()` so the build record is updated correctly

---

## Item 4 — Truncate error field; keep logs in K8s

**Status:** [x] done

`w.fail(..., fmt.Errorf("build job failed\n\n<full K8s logs>"))` stores potentially hundreds of lines into the `error` DB column. Logs are already retrievable via `/builds/:build_id/logs`.

- In `runBuildKitJob`, on job failure extract only the last meaningful error line(s) from logs (e.g. first non-empty line from the failed container, max 500 chars) as the error reason
- Full logs remain fetchable from K8s via the existing logs endpoint
- Define a helper `extractBuildError(logs string) string` that returns a concise reason

---

## Item 5 — Cancel / clean up K8s job on context cancellation

**Status:** [x] done

When the River job context is cancelled (server restart, timeout), `runBuildKitJob` exits with `ctx.Err()` but the K8s Job keeps running until TTL (1 hour). The build record is marked `failed` but wasted compute continues.

- In `runBuildKitJob`, before returning any error, delete the K8s Job using a background context:
  ```go
  defer func() {
      _ = clientset.BatchV1().Jobs(ns).Delete(context.Background(), jobName, metav1.DeleteOptions{
          PropagationPolicy: &deleteBackground,
      })
  }()
  ```
  Place this defer immediately after the job is created.
- This ensures cleanup on both normal failure and context cancellation paths.

---

## Item 6 — Use owner reference on token secret

**Status:** [x] done

The `defer Secrets.Delete(...)` in `runBuildKitJob` only runs when the function returns normally. A hard process kill leaves the token secret in K8s permanently.

- Set a K8s `OwnerReference` on the token secret pointing to the Job object, so K8s garbage-collects it automatically when the Job is deleted (via TTL or the defer added in Item 5).
- Remove the explicit `defer Secrets.Delete` — owner reference makes it redundant.

---

## Item 7 — Retry transient errors; cancel on permanent failures

**Status:** [ ] pending

`MaxAttempts: 1` means any blip (Pipes token fetch timeout, K8s API unavailable) permanently fails the build.

- Change `MaxAttempts` to `3`
- Classify errors:
  - Permanent (return `river.JobCancel(err)`): missing astropods.yml, empty agent name, build job failed (bad Dockerfile/code)
  - Retriable (return plain `err`): token fetch failure, K8s API errors, context deadline on infrastructure ops
- River will retry retriable errors with backoff; permanent errors skip remaining attempts

---

## Item 8 — Ensure build namespace/SA at startup, not per-build

**Status:** [ ] pending

`Namespaces().Create()` and `ServiceAccounts().Create()` are called on every build. While `IsAlreadyExists` short-circuits them, it's a wasted round-trip pair on every job.

- Add a `EnsureBuildInfrastructure(ctx context.Context) error` method to `GitHubBuildWorker` (or call it from server startup)
- Call it once during server initialisation, not inside `runBuildKitJob`
- Remove the create calls from `runBuildKitJob`

---

## Item 9 — Pin init container image versions

**Status:** [ ] pending

`alpine/git:latest` and `amazon/aws-cli:latest` will silently change and can break builds.

- Pin `alpine/git` to a specific semver tag (e.g. `alpine/git:2.47.2`)
- Pin `amazon/aws-cli` to a specific version (e.g. `amazon/aws-cli:2.24.21`)
- Define both as named constants at the top of `github_build.go`, alongside `buildKitImage`

---

## Item 10 — Replace wall-clock poll deadline with context timeout

**Status:** [ ] pending

`deadline := time.Now().Add(20 * time.Minute)` runs independently of the River job context. Cancellation is only checked at the top of each poll iteration, not at the deadline boundary.

- Replace the wall-clock deadline with `context.WithTimeout(ctx, 20*time.Minute)`
- The `select` in the loop listens on both `ctx.Done()` and the tick; a timeout or external cancel both surface as `ctx.Err()`
- Simplifies the loop: no manual deadline variable needed
