# Force-refresh the messaging sidecar pull-through cache from Queen

## Summary

Deployed agents run the messaging sidecar from the account's ECR Docker Hub pull-through cache (`{ecrHost}/dockerhub/astropods/messaging:latest`). ECR re-checks Docker Hub for a given tag at most once every ~24 hours, and that window is AWS-managed — not configurable in our code or Terraform. So a freshly published `astropods/messaging:latest` could sit undelivered for up to a day. This adds an admin action in Queen to force that refresh on demand.

## Design

Queen holds no AWS credentials — it is a thin local UI that proxies over mTLS to astro-server's `AdminService`. The refresh therefore lives on astro-server (which has IRSA-backed AWS access) and is exposed to Queen as a new RPC.

- **AdminService.RefreshMessagingCache** — new gRPC method. astro-server evicts the cached tag via the ECR API (`BatchDeleteImage` on repository `dockerhub/astropods/messaging`, tag `latest`). Deleting the tag makes the next agent pull a fresh import from Docker Hub, bypassing the 24h timer. A missing repository or tag is treated as success (nothing cached → the next pull is already fresh); ECR reports a missing tag as a per-image failure rather than a call error, so that case is handled explicitly. The call is audit-logged (`image_cache.refresh_messaging`).
- **imagecache helper** — a small `internal/imagecache` package builds the ECR client from the pod's default AWS config (same pattern as the GitHub build path) and performs the eviction. The messaging repo/tag are fixed by design.
- **Queen proxy + UI** — a `POST /api/admin/refresh-messaging-cache` handler forwards to the RPC. The Clusters page gains a "Refresh messaging cache" button gated behind a confirmation dialog (project `AlertDialog`, not a browser confirm) that surfaces the returned status.

This action only evicts the cache. Running agents keep their current sidecar until their pods restart or redeploy, because the deployment spec pins the mutable `:latest` tag and pods only re-pull on restart. A fleet-wide restart is intentionally out of scope.

The astro-server IRSA role (`{env}-astro-server`) previously had no permissions on the `dockerhub/*` pull-through repos. A statement granting `ecr:BatchDeleteImage`, `ecr:DescribeImages`, and `ecr:ListImages` on `repository/dockerhub/*` was added to the preview and prod policies. Node roles already hold `ecr:BatchImportUpstreamImage` on `dockerhub/*`, so the re-import needs no additional grant.

## Migration

Apply the astro-infra Terraform IAM change (preview and prod) before the action will work in those environments — without the new ECR permissions the RPC returns an AWS authorization error. No changes are required for deployed agents; the new sidecar is picked up on their next restart or redeploy.
