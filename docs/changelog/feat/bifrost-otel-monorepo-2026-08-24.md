# Build and deploy bifrost-otel from the monorepo

## Summary

`bifrost-otel` is the collector that turns Bifrost's GenAI spans into Metronome
usage events. It lived in astro-infra, where nothing built its image and nothing
rolled its deployment, so shipping it meant building by hand and restarting the
pods from the bastion.

That is why astro-infra#77, which fixed gateway usage being billed per trace
instead of per request, sat merged and undeployed. Preview is capturing 54.6% of
gateway cost while the fix waits: $36.45 of $66.79 across the three accounts with
real spend, measured over the current billing period.

Saswat put it in astro-infra while testing and agreed to move it here.

## Design

**Only the image source moves.** The chart, values, and Terraform stay in
astro-infra. The chart already reads `module.ecr.repository_urls["bifrost-otel"]`
with `tag: latest` and `pullPolicy: Always`, and the ECR module names
repositories `${environment}-${name}`, which is exactly what CI pushes to. So
nothing in Terraform changes.

**The exporter is unchanged.** All four Go files are byte-identical to
astro-infra `main`, apart from `//nolint:gosec` on five OTel attribute-name
constants: gosec reads `gen_ai.usage.input_tokens` as a credential, and this
package has never been linted because astro-infra has no golangci config. Only
`builder-config.yaml` and the exporter's `go.mod` change, to move the module path
from `github.com/astropods/astro-infra/bifrost-otel` to
`github.com/astropods/astro/apps/bifrost-otel`.

**The Dockerfile cannot cache dependencies the way the others do.** OCB generates
a main package and a `go.mod` under `output_path` and then compiles them, so the
whole tree has to be present before the build runs. There is no go.mod-first
layer to copy. `GOOS`/`GOARCH` reach OCB's compile step, verified by building
`linux/amd64` on arm64 and confirming an x86-64 static binary.

**The testable module is the vendored exporter, not the project root.** The
distribution's `go.mod` does not exist until OCB runs, so the Go test matrix
points at `apps/bifrost-otel/internal/exporter/bifrostotel`.

## Migration

Merging this builds and pushes the image. It does not deploy it yet: the
`bifrost-otel` chart has no Keel annotation, so nothing watches the tag and the
running pods keep the image they have. `astro-otel` has one:

```yaml
keel:
  pollSchedule: "@every 1m"
```

The paired astro-infra change adds the same block and deletes
`images/bifrost-otel`. Until it lands, deploying still means restarting the
deployment by hand, so land both together.

Nothing else changes: the collector receives the same OTLP traces on the same
ports and writes the same `ai_gateway_llm_usage` events.
