## Summary

BuildKit image build jobs were failing to schedule due to competing with agent workloads for cluster capacity. This change isolates build jobs to a dedicated node group and adds ECR registry-based layer caching to speed up repeat builds.

## Design

**Node isolation** — Build jobs now require `workload-type=build` node label and tolerate the `astro.dev/build:NoSchedule` taint. This means build jobs only land on dedicated build nodes, and those nodes repel all other workloads. The target node type is c5d.xlarge/c5d.2xlarge (NVMe instance store) for fast layer I/O.

**ECR layer caching** — Each build job now passes `--import-cache` and `--export-cache` pointing at a stable `:cache` tag in the same ECR repository. BuildKit uses `mode=max` to store all intermediate layers, not just the final image. The import is best-effort — BuildKit silently skips it if no cache exists yet.

```
# cache tag derived from destination
<ecr-repo>:<buildID>  →  <ecr-repo>:cache
```

**BuildKit ConfigMap** — A new `GITHUB_BUILDKIT_CONFIGMAP` env var (optional) names a ConfigMap in the build namespace. When set, it is mounted at `/etc/buildkit` and `--config=/etc/buildkit/buildkitd.toml` is appended to `BUILDKITD_FLAGS`. This enables custom daemon config such as pointing the build root at an NVMe instance store (`/mnt/nvme/buildkit`).

## Migration

No action required for existing deployments. To enable custom buildkitd config, create a ConfigMap with a `buildkitd.toml` key and set `GITHUB_BUILDKIT_CONFIGMAP=<name>` on astro-server.

The new node selector and taint require a dedicated build node group to be provisioned with the matching label and taint before build jobs will schedule.
