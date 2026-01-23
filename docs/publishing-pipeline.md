# Astro Publishing and Scanning Pipeline

## Overview

The publishing pipeline validates, secures, and optimizes agent artifacts before making them available in the registry. It ensures quality, security, and consistency across all published agents.

## Pipeline Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    astro publish                             │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 1: Pre-flight Validation                             │
│  - Artifact integrity check                                 │
│  - Spec schema validation                                   │
│  - Required fields validation                               │
│  - Version conflict check                                   │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 2: Artifact Unpacking & Analysis                     │
│  - Extract artifact contents                                │
│  - Analyze container image                                  │
│  - Parse knowledge base                                     │
│  - Extract metadata                                         │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 3: Container Image Processing                        │
│  - Pull container image (if not bundled)                    │
│  - Multi-arch validation                                    │
│  - Push to registry namespace                               │
│  - Store image reference                                    │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 4: Security Scanning                                 │
│  ├─ Container vulnerability scan (Trivy/Grype)              │
│  ├─ Secret detection                                        │
│  ├─ License compliance check                                │
│  ├─ Malware scanning                                        │
│  └─ SBOM generation                                         │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 5: Content Validation                                │
│  ├─ Knowledge base validation                               │
│  ├─ Model configuration validation                          │
│  ├─ Tool specification validation                           │
│  └─ Documentation check (README, changelog)                 │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 6: Artifact Signing (Optional)                       │
│  - Sign artifact with private key                           │
│  - Generate signature                                       │
│  - Attach signature to manifest                             │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 7: OCI Artifact Assembly                             │
│  - Create OCI manifest                                      │
│  - Upload layers to registry                                │
│  - Add annotations and metadata                             │
│  - Calculate digests                                        │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 8: Metadata Indexing                                 │
│  - Update search index                                      │
│  - Generate catalog entry                                   │
│  - Update statistics                                        │
│  - Trigger notifications                                    │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Stage 9: Post-Publishing                                   │
│  - Generate artifact URL                                    │
│  - Send webhooks                                            │
│  - Update registry UI                                       │
│  - Audit log entry                                          │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
                  Published! ✓
```

## Stage 1: Pre-flight Validation

### Artifact Integrity Check

```bash
# Verify artifact file integrity
$ astro publish my-agent.astro
[1/9] Pre-flight validation...
  ✓ Artifact file exists
  ✓ Artifact is readable
  ✓ Checksum validation passed
```

**Checks:**
- File exists and is readable
- Checksum matches (if provided)
- File is valid tar/gzip archive
- Not corrupted

### Spec Schema Validation

```bash
[1/9] Pre-flight validation...
  ✓ Parsing astro-spec.yml
  ✓ Schema version: astro.dev/v1
  ✓ All required fields present
  ✓ Field types valid
```

**Validates:**
```yaml
apiVersion: astro.dev/v1  # Must be valid version
kind: AgentInfrastructure  # Must be valid kind
metadata:
  name: "..."  # Required, valid format
  version: "..."  # Required, semver format
runtime:
  image: "..." # OR build context
models:
  primary: # At least one model required
    name: "..."
    provider: "..."
```

**Validation Rules:**
- `metadata.name`: lowercase, alphanumeric, hyphens only, max 63 chars
- `metadata.version`: valid semver (e.g., 1.2.3, 1.0.0-beta)
- `metadata.author`: non-empty string
- `runtime.image` OR `runtime.build` must be present
- Models, knowledge, tools configs valid according to schema

### Version Conflict Check

```bash
[1/9] Pre-flight validation...
  ✓ Checking version conflicts...
  ✗ Version 1.2.0 already exists
  ℹ Use --force to overwrite (not recommended)
```

**Checks:**
- Version doesn't already exist in registry
- If exists, require `--force` flag
- Semantic version is higher than latest (warning if not)

### Authentication & Authorization

```bash
[1/9] Pre-flight validation...
  ✓ Authenticated as: user@example.com
  ✓ Permission check: myorg/my-agent
  ✓ Quota check: 45/100 agents used
```

**Checks:**
- User is authenticated (has valid token)
- User has permission to publish to namespace `myorg/my-agent`
- Organization quota not exceeded
- Rate limits not hit

## Stage 2: Artifact Unpacking & Analysis

### Extract Artifact

```bash
[2/9] Analyzing artifact...
  ✓ Extracting artifact: my-agent.astro
  ✓ Found astro-spec.yml
  ✓ Found runtime/image-ref.json
  ✓ Found knowledge/ (105 MB)
  ✓ Found models/config.json
  ✓ Found tools/
```

**Extracts:**
```
temp-workspace/
├── astro-spec.yml
├── manifest.json
├── runtime/
│   ├── image-ref.json
│   └── image.tar (if bundled)
├── knowledge/
│   ├── schema.json
│   └── indices/
├── models/
│   └── config.json
├── tools/
│   ├── tool1.spec.json
│   └── tool2.spec.json
├── interface/
│   └── openapi.yaml
└── observability/
    └── traces.config.json
```

### Analyze Container Image

```bash
[2/9] Analyzing artifact...
  ✓ Container image: ghcr.io/myorg/agent:1.2.0
  ✓ Checking image existence...
  ✓ Image found in registry
  ✓ Architecture: linux/amd64
  ✓ Size: 245 MB
  ✓ Layers: 12
```

**Analysis:**
- Image reference valid and accessible
- Multi-architecture support detected
- Image size and layer count
- Base image information
- Entry point and command

**Image Metadata Extracted:**
```json
{
  "image": "ghcr.io/myorg/agent:1.2.0",
  "digest": "sha256:abc123...",
  "architecture": ["linux/amd64", "linux/arm64"],
  "size": 257698304,
  "layers": 12,
  "base_image": "node:20-alpine",
  "created": "2026-01-13T10:00:00Z"
}
```

### Parse Knowledge Base

```bash
[2/9] Analyzing artifact...
  ✓ Knowledge base found
  ✓ Collections: product-docs, faq
  ✓ Total documents: 1,247
  ✓ Total embeddings: 3,891
  ✓ Vector dimension: 1536
  ✓ Storage size: 105 MB
```

**Analyzes:**
- Collection names and counts
- Document count per collection
- Embedding dimensions
- Vector database schema
- Storage requirements

### Extract Metadata

```bash
[2/9] Analyzing artifact...
  ✓ Extracting metadata...
    - Name: customer-support-agent
    - Version: 1.2.0
    - Author: Support Team
    - License: MIT
    - Models: gpt-4-turbo, text-embedding-3-large
    - Tools: zendesk, stripe
    - Tags: customer-service, rag, support
```

## Stage 3: Container Image Processing

### Pull Container Image

```bash
[3/9] Processing container image...
  ✓ Pulling ghcr.io/myorg/agent:1.2.0
  ✓ Pulling linux/amd64 variant
  ✓ Pulling linux/arm64 variant
  ✓ Pull complete (245 MB)
```

**Process:**
- Pull all architecture variants
- Verify digest matches manifest
- Calculate total size
- Store in temporary cache

### Multi-arch Validation

```bash
[3/9] Processing container image...
  ✓ Validating multi-arch manifest
  ✓ linux/amd64: present
  ✓ linux/arm64: present
  ✓ All variants use same base layers
```

**Validates:**
- Multi-arch manifest is properly formatted
- All declared architectures present
- Base layers consistent across architectures
- No architecture-specific bugs

### Push to Registry Namespace

```bash
[3/9] Processing container image...
  ✓ Retagging for registry namespace
  ✓ Pushing to registry.astro.dev/myorg/agent:1.2.0
  ✓ Manifest uploaded
  ✓ Image reference stored
```

**Process:**
1. Retag image with registry namespace
2. Push to Astro registry (or mirror)
3. Link image to agent artifact
4. Store digest for verification

**Why copy to Astro registry?**
- Ensures availability (source registry might go down)
- Faster pulls for users
- Consistent namespace management
- Allows registry-side optimizations

## Stage 4: Security Scanning

### Container Vulnerability Scan

```bash
[4/9] Security scanning...
  ⏳ Scanning container image (ghcr.io/myorg/agent:1.2.0)
  ✓ Scan complete (45 seconds)

  Vulnerabilities found:
    Critical: 0
    High:     1
    Medium:   3
    Low:      12

  Details:
    - urllib3 (medium): CVE-2023-43804
    - openssl (high): CVE-2024-0001
    - jq (low): CVE-2023-12345
```

**Scanning with Trivy:**
```bash
# Internally runs
trivy image --format json ghcr.io/myorg/agent:1.2.0
```

**Scans for:**
- OS package vulnerabilities (Alpine, Debian, Ubuntu, etc.)
- Application dependency vulnerabilities (npm, pip, gem, etc.)
- Known CVEs with severity scores
- Outdated packages

**Policy Enforcement:**
```yaml
# Registry policy
scanning:
  block_on_critical: true  # Block if critical vulns found
  block_on_high: false     # Warn but allow
  max_medium: 10           # Block if > 10 medium vulns
```

**Output:**
```bash
[4/9] Security scanning...
  ⚠ Warning: 1 high severity vulnerability found
  ℹ Review report: https://registry.astro.dev/reports/scan-abc123
  ℹ Publishing will proceed (not blocking)
```

### Secret Detection

```bash
[4/9] Security scanning...
  ✓ Scanning for secrets...
  ✗ Potential secret detected!

  File: /app/.env.example
  Line: 3
  Type: AWS Access Key
  Value: AKIA****************

  ❌ Publishing blocked. Remove secrets and try again.
```

**Scans for:**
- API keys (AWS, GCP, Azure, OpenAI, etc.)
- Private keys (RSA, SSH, PGP)
- Database credentials
- Tokens and passwords
- Certificate files

**Tools:**
- **gitleaks**: Git secret scanner
- **trufflehog**: Secret hunting tool
- Custom patterns for agent-specific secrets

**Allowlist:**
```yaml
# .astro/secret-allowlist.yaml
allowlist:
  - path: .env.example
    reason: Example file with fake credentials
  - path: tests/fixtures/test-key.pem
    reason: Test fixture key
```

### License Compliance Check

```bash
[4/9] Security scanning...
  ✓ Checking license compliance...
  ✓ Container image license: MIT
  ✓ Dependency licenses:
    - express (MIT)
    - axios (MIT)
    - dotenv (BSD-2-Clause)
  ⚠ Warning: 2 dependencies with copyleft licenses
    - readline (GPL-3.0)
    - glibc (LGPL-2.1)
```

**Checks:**
- Container image license matches declared license
- All dependencies have compatible licenses
- No copyleft licenses (GPL) unless declared
- License files present

**Policy:**
```yaml
licensing:
  allowed_licenses:
    - MIT
    - Apache-2.0
    - BSD-2-Clause
    - BSD-3-Clause
    - ISC
  warn_licenses:
    - LGPL-2.1
    - LGPL-3.0
  blocked_licenses:
    - GPL-2.0
    - GPL-3.0  # Unless explicitly allowed
```

### Malware Scanning

```bash
[4/9] Security scanning...
  ✓ Scanning for malware...
  ✓ No malware detected
  ✓ No suspicious patterns found
```

**Scans for:**
- Known malware signatures (ClamAV)
- Cryptocurrency miners
- Backdoors and trojans
- Suspicious shell scripts
- Obfuscated code

### SBOM Generation

```bash
[4/9] Security scanning...
  ✓ Generating Software Bill of Materials (SBOM)
  ✓ Format: SPDX 2.3
  ✓ Components: 127
  ✓ SBOM saved: sbom-abc123.spdx.json
```

**SBOM Contents:**
```json
{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "customer-support-agent-1.2.0",
  "packages": [
    {
      "SPDXID": "SPDXRef-Package-express",
      "name": "express",
      "versionInfo": "4.18.2",
      "licenseConcluded": "MIT",
      "externalRefs": [
        {
          "referenceCategory": "PACKAGE-MANAGER",
          "referenceType": "purl",
          "referenceLocator": "pkg:npm/express@4.18.2"
        }
      ]
    }
  ]
}
```

**Used for:**
- Vulnerability tracking over time
- License compliance auditing
- Supply chain security
- Dependency analysis

## Stage 5: Content Validation

### Knowledge Base Validation

```bash
[5/9] Validating content...
  ✓ Validating knowledge base...
  ✓ Schema is valid
  ✓ All collections present
  ✓ Embeddings dimension: 1536
  ✓ No empty collections
  ✓ Metadata fields consistent
```

**Validates:**
- Vector DB schema is valid for provider (Qdrant, Pinecone, etc.)
- All declared collections exist
- Embedding dimensions consistent
- No empty or malformed collections
- Metadata schema matches specification

**Example Validation:**
```yaml
# In spec
knowledge:
  vector_db:
    provider: qdrant
    collection: support-docs
    schema:
      vector_size: 1536
      distance: cosine
    indices:
      - name: product-docs
      - name: faq
```

**Checks:**
- `indices/product-docs.snapshot` exists
- `indices/faq.snapshot` exists
- Vector dimensions = 1536
- Distance metric = cosine

### Model Configuration Validation

```bash
[5/9] Validating content...
  ✓ Validating model configurations...
  ✓ Primary model: gpt-4-turbo (OpenAI)
  ✓ Embedding model: text-embedding-3-large (OpenAI)
  ✓ Model compatibility verified
  ✓ Estimated cost: $0.03/1K tokens
```

**Validates:**
- Model names are valid for provider
- Provider credentials will be required (documented)
- Model configurations are compatible
- Cost estimates are reasonable

**Checks for common issues:**
- Typos in model names (gpt4-turbo vs gpt-4-turbo)
- Invalid provider combinations
- Embedding dimensions mismatch
- Deprecated models

### Tool Specification Validation

```bash
[5/9] Validating content...
  ✓ Validating tool specifications...
  ✓ Tool: zendesk (REST API)
    - Base URL: https://company.zendesk.com/api/v2
    - Endpoints: 2
    - Auth: Bearer token
  ✓ Tool: stripe (REST API)
    - Base URL: https://api.stripe.com/v1
    - Endpoints: 2
    - Auth: Bearer token
  ✓ All tool specs valid
```

**Validates:**
- Tool types are supported (rest_api, graphql, grpc, function)
- Base URLs are valid and use HTTPS
- Endpoint paths are properly formatted
- Authentication methods are supported
- Required fields present

### Documentation Check

```bash
[5/9] Validating content...
  ✓ Checking documentation...
  ✓ README.md found (2.4 KB)
  ✓ CHANGELOG.md found (1.1 KB)
  ⚠ No LICENSE file found (using metadata license)
  ✓ Interface spec (OpenAPI) present
```

**Checks:**
- README.md exists and non-empty
- Minimum README content (description, usage)
- CHANGELOG.md for version history
- LICENSE file matches declared license
- Interface documentation (OpenAPI/gRPC proto)

**Scoring for "Verified" Badge:**
```
Documentation Score:
  ✓ README with examples: +20 points
  ✓ CHANGELOG maintained: +10 points
  ✓ LICENSE file: +10 points
  ✓ OpenAPI/gRPC spec: +15 points
  ✓ Example usage: +15 points
  ✓ Architecture diagram: +10 points
  ✓ Troubleshooting guide: +10 points
  ✓ Contributing guide: +10 points

Total: 85/100 (Good)
```

## Stage 6: Artifact Signing

### Generate Signature

```bash
[6/9] Signing artifact...
  ✓ Loading private key: ~/.astro/keys/myorg-signing-key
  ✓ Computing artifact digest
  ✓ Signing with RSA-SHA256
  ✓ Signature generated
```

**Signing Process:**

1. **Compute Digest:**
```bash
# Calculate SHA256 of artifact manifest
digest=$(sha256sum manifest.json | awk '{print $1}')
# digest: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

2. **Sign Digest:**
```bash
# Sign with private key
echo -n "$digest" | openssl dgst -sha256 -sign private-key.pem | base64
# signature: MEUCIQDxxx...yyy
```

3. **Attach to Manifest:**
```json
{
  "annotations": {
    "org.astro.signature.v1": "MEUCIQDxxx...yyy",
    "org.astro.signature.keyId": "myorg-signing-key-2024",
    "org.astro.signature.algorithm": "RSA-SHA256",
    "org.astro.signature.timestamp": "2026-01-13T10:30:00Z"
  }
}
```

### Store Public Key Reference

```bash
[6/9] Signing artifact...
  ✓ Storing public key reference
  ✓ Key ID: myorg-signing-key-2024
  ✓ Fingerprint: AA:BB:CC:DD:EE:FF...
```

**Public Key Storage:**
- Stored in registry for verification
- Associated with organization
- Fingerprint used for key identification
- Multiple keys can be active (rotation)

### Verification Info

```bash
[6/9] Signing artifact...
  ✓ Signature attached to artifact
  ℹ Users can verify with:
    astro verify myorg/my-agent:1.2.0
  ℹ Public key available at:
    https://registry.astro.dev/v1/keys/myorg/myorg-signing-key-2024.pem
```

## Stage 7: OCI Artifact Assembly

### Create OCI Manifest

```bash
[7/9] Assembling OCI artifact...
  ✓ Creating OCI manifest
  ✓ Artifact type: application/vnd.astro.agent.v1
  ✓ Config blob: 1.2 KB
  ✓ Layers: 5
```

**OCI Manifest Structure:**
```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.astro.agent.v1",
  "config": {
    "mediaType": "application/vnd.astro.agent.config.v1+json",
    "digest": "sha256:abc123...",
    "size": 1234
  },
  "layers": [
    {
      "mediaType": "application/vnd.astro.spec.v1+yaml",
      "digest": "sha256:def456...",
      "size": 5678
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:789abc...",
      "size": 2345,
      "annotations": {
        "org.astro.container.image": "ghcr.io/myorg/agent:1.2.0"
      }
    },
    {
      "mediaType": "application/vnd.astro.knowledge.v1.tar+gzip",
      "digest": "sha256:012def...",
      "size": 110100480
    },
    {
      "mediaType": "application/vnd.astro.models.v1+json",
      "digest": "sha256:345678...",
      "size": 890
    },
    {
      "mediaType": "application/vnd.astro.tools.v1.tar+gzip",
      "digest": "sha256:678901...",
      "size": 4567
    }
  ]
}
```

### Upload Layers

```bash
[7/9] Assembling OCI artifact...
  ✓ Uploading layers to registry...
  [1/5] astro-spec.yml (5.6 KB)           ████████████ 100%
  [2/5] container-image-ref (2.3 KB)      ████████████ 100%
  [3/5] knowledge.tar.gz (105 MB)         ████████████ 100%
  [4/5] models-config.json (890 B)        ████████████ 100%
  [5/5] tools-specs.tar.gz (4.5 KB)       ████████████ 100%
  ✓ All layers uploaded
```

**Upload Process:**
1. Chunk large layers (> 5MB) for resumable upload
2. Calculate digest for each layer
3. Check if layer exists (deduplication)
4. Upload missing layers only
5. Verify upload with HEAD request

### Add Annotations

```bash
[7/9] Assembling OCI artifact...
  ✓ Adding metadata annotations...
  ✓ Agent metadata
  ✓ Security scan results
  ✓ Signature information
  ✓ Build information
```

**Annotations Added:**
```json
{
  "annotations": {
    // Agent metadata
    "org.astro.agent.name": "customer-support-agent",
    "org.astro.agent.version": "1.2.0",
    "org.astro.agent.description": "Customer support agent with RAG",
    "org.astro.agent.author": "Support Team",
    "org.astro.agent.license": "MIT",
    "org.astro.agent.homepage": "https://github.com/myorg/support-agent",
    "org.astro.agent.tags": "customer-service,rag,support",

    // Requirements
    "org.astro.agent.models": "gpt-4-turbo,text-embedding-3-large",
    "org.astro.agent.vector_db": "qdrant",
    "org.astro.agent.tools": "zendesk,stripe",

    // Security
    "org.astro.scan.vulnerabilities.critical": "0",
    "org.astro.scan.vulnerabilities.high": "1",
    "org.astro.scan.vulnerabilities.medium": "3",
    "org.astro.scan.timestamp": "2026-01-13T10:35:00Z",
    "org.astro.signature.v1": "MEUCIQDxxx...",

    // Build info
    "org.astro.build.timestamp": "2026-01-13T10:30:00Z",
    "org.astro.build.version": "0.1.0",
    "org.astro.build.platform": "linux/amd64"
  }
}
```

### Calculate Final Digest

```bash
[7/9] Assembling OCI artifact...
  ✓ Calculating manifest digest...
  ✓ Digest: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Stage 8: Metadata Indexing

### Update Search Index

```bash
[8/9] Indexing metadata...
  ✓ Updating search index...
  ✓ Indexing agent name
  ✓ Indexing description
  ✓ Indexing tags
  ✓ Indexing tools and models
```

**Search Index (Elasticsearch/OpenSearch):**
```json
{
  "agent_id": "myorg/customer-support-agent",
  "version": "1.2.0",
  "name": "customer-support-agent",
  "org": "myorg",
  "description": "Customer support agent with RAG over documentation",
  "author": "Support Team",
  "tags": ["customer-service", "rag", "support"],
  "models": ["gpt-4-turbo", "text-embedding-3-large"],
  "tools": ["zendesk", "stripe"],
  "vector_db": "qdrant",
  "license": "MIT",
  "created": "2026-01-13T10:30:00Z",
  "downloads": 0,
  "stars": 0,
  "verified": false,
  "has_vulnerabilities": true,
  "vulnerability_severity": "high"
}
```

**Indexed Fields:**
- Full-text: name, description, author
- Keywords: tags, models, tools, license
- Facets: vector_db, has_vulnerabilities, verified
- Sort: downloads, stars, created

### Generate Catalog Entry

```bash
[8/9] Indexing metadata...
  ✓ Generating catalog entry...
  ✓ Added to myorg namespace
  ✓ Version 1.2.0 is now latest
```

**Catalog Entry:**
```json
{
  "name": "myorg/customer-support-agent",
  "latest": "1.2.0",
  "versions": ["1.0.0", "1.1.0", "1.2.0"],
  "created": "2025-10-01T00:00:00Z",
  "updated": "2026-01-13T10:30:00Z",
  "downloads_total": 15234,
  "downloads_last_30d": 2341,
  "stars": 142,
  "verified": false
}
```

### Update Statistics

```bash
[8/9] Indexing metadata...
  ✓ Updating statistics...
  ✓ Organization agent count: 8
  ✓ Total registry agents: 1,247
```

**Statistics Tracked:**
- Total agents in registry
- Agents per organization
- Versions per agent
- Total downloads
- Active users

### Trigger Notifications

```bash
[8/9] Indexing metadata...
  ✓ Sending notifications...
  ✓ Webhook: https://hooks.slack.com/... (200 OK)
  ✓ Email notification queued
  ✓ RSS feed updated
```

**Notifications:**
- **Webhooks**: Slack, Discord, custom URLs
- **Email**: Followers of the agent
- **RSS**: Registry activity feed
- **GitHub**: Release notification (if linked)

**Webhook Payload:**
```json
{
  "event": "agent.published",
  "agent": {
    "name": "myorg/customer-support-agent",
    "version": "1.2.0",
    "url": "https://registry.astro.dev/myorg/customer-support-agent"
  },
  "publisher": {
    "username": "alice",
    "email": "alice@example.com"
  },
  "timestamp": "2026-01-13T10:30:00Z"
}
```

## Stage 9: Post-Publishing

### Generate URLs

```bash
[9/9] Finalizing...
  ✓ Artifact URL: https://registry.astro.dev/myorg/customer-support-agent:1.2.0
  ✓ Pull command: astro pull myorg/customer-support-agent:1.2.0
  ✓ Install command: astro install myorg/customer-support-agent:1.2.0
  ✓ Web UI: https://registry.astro.dev/agents/myorg/customer-support-agent
```

### Update Registry UI

```bash
[9/9] Finalizing...
  ✓ Updating registry web UI...
  ✓ Agent page refreshed
  ✓ Organization page updated
  ✓ Search index synchronized
```

### Audit Log

```bash
[9/9] Finalizing...
  ✓ Audit log entry created
  ✓ Event ID: evt_abc123def456
```

**Audit Log Entry:**
```json
{
  "event_id": "evt_abc123def456",
  "event_type": "agent.published",
  "timestamp": "2026-01-13T10:30:00Z",
  "actor": {
    "user_id": "user_123",
    "username": "alice",
    "email": "alice@example.com",
    "ip_address": "203.0.113.42"
  },
  "resource": {
    "type": "agent",
    "name": "myorg/customer-support-agent",
    "version": "1.2.0",
    "digest": "sha256:e3b0c442..."
  },
  "metadata": {
    "artifact_size": 110558208,
    "scan_results": {
      "vulnerabilities": {
        "critical": 0,
        "high": 1,
        "medium": 3
      }
    },
    "signed": true
  }
}
```

### Success Output

```bash
✅ Published successfully!

Agent:    myorg/customer-support-agent:1.2.0
Digest:   sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
Size:     105.4 MB
Signed:   Yes
Scanned:  Yes (⚠ 1 high, 3 medium vulnerabilities)

URLs:
  Registry: https://registry.astro.dev/myorg/customer-support-agent
  Pull:     astro pull myorg/customer-support-agent:1.2.0
  Install:  astro install myorg/customer-support-agent:1.2.0

Security Report: https://registry.astro.dev/reports/scan-abc123

⚠ Security Advisory:
  1 high severity vulnerability found in openssl
  Consider updating dependencies before production deployment.
  View details: https://registry.astro.dev/reports/scan-abc123
```

## Error Handling

### Validation Failures

```bash
[1/9] Pre-flight validation...
  ✗ Invalid spec: metadata.version must be valid semver
  ✗ Found: "v1.2.0"
  ✓ Expected: "1.2.0" (without 'v' prefix)

❌ Publishing failed. Fix errors and try again.
```

### Scan Failures

```bash
[4/9] Security scanning...
  ✗ Critical vulnerability detected!

  Package:  openssl
  Version:  1.1.1k
  CVE:      CVE-2024-99999
  Severity: Critical (9.8)
  Fixed in: 3.0.0

❌ Publishing blocked due to critical vulnerability.

Recommended actions:
  1. Update openssl to 3.0.0 or later
  2. Rebuild container image
  3. Run: astro build --scan to verify fix
  4. Retry publishing
```

### Secret Detection

```bash
[4/9] Security scanning...
  ✗ Secret detected in artifact!

  File:   src/config.ts
  Line:   12
  Type:   OpenAI API Key
  Value:  sk-proj-*********************

❌ Publishing blocked. Secrets must not be included in artifacts.

How to fix:
  1. Remove secret from source code
  2. Use environment variables instead
  3. Update astro-spec.yml to reference secret:

     environment:
       - name: OPENAI_API_KEY
         valueFrom:
           secretRef: openai-api-key

  4. Rebuild and retry
```

### Network Failures

```bash
[3/9] Processing container image...
  ✗ Failed to pull ghcr.io/myorg/agent:1.2.0
  ✗ Error: dial tcp: lookup ghcr.io: no such host

❌ Publishing failed. Check network connection and retry.

If issue persists:
  - Verify image exists: docker pull ghcr.io/myorg/agent:1.2.0
  - Check registry credentials
  - Try: astro publish --retry
```

### Quota Exceeded

```bash
[1/9] Pre-flight validation...
  ✗ Organization quota exceeded
  ✗ Current: 100/100 agents
  ✗ Upgrade required to publish more agents

❌ Publishing failed. Upgrade your plan or delete unused agents.

Options:
  - Upgrade: https://registry.astro.dev/settings/billing
  - Delete agents: astro delete <agent-name>
  - Contact support for quota increase
```

## Retry and Resume

### Automatic Retry

```bash
[3/9] Processing container image...
  ✗ Upload failed: connection reset
  ⏳ Retrying (1/3)...
  ✓ Upload successful
```

**Retry Strategy:**
- Exponential backoff: 1s, 2s, 4s, 8s, 16s
- Max retries: 3
- Only retry transient errors (network, timeout)
- Don't retry permanent errors (auth, validation)

### Resumable Upload

```bash
[7/9] Assembling OCI artifact...
  ⏳ Uploading knowledge.tar.gz (105 MB)
  ████████░░░░ 67% (70 MB)
  ✗ Connection lost

Publishing paused. Resume with:
  astro publish --resume abc123
```

**Resume Process:**
1. Store upload state in ~/.astro/uploads/
2. Track uploaded chunks
3. Resume from last successful chunk
4. Verify integrity after resume

## Performance Optimizations

### Layer Deduplication

```bash
[7/9] Assembling OCI artifact...
  ✓ Checking layer deduplication...
  ✓ astro-spec.yml: new
  ✓ container-image-ref: new
  ✓ knowledge.tar.gz: exists (skipped upload)
  ✓ models-config.json: new
  ✓ tools-specs.tar.gz: new

  Saved: 105 MB (50% of total)
```

### Parallel Scanning

```bash
[4/9] Security scanning...
  ⏳ Running scans in parallel...
  ├─ Container scan (Trivy)       ████████████ Done
  ├─ Secret detection (gitleaks)  ████████████ Done
  ├─ License check (licensei)     ████████████ Done
  └─ SBOM generation (syft)       ████████████ Done

  ✓ All scans complete (32 seconds)
```

### Incremental Updates

```bash
[8/9] Indexing metadata...
  ✓ Using incremental index update
  ✓ Only changed fields updated
  ✓ Search latency: <5ms
```

## Monitoring and Metrics

### Pipeline Metrics

```
Publishing Pipeline Metrics (Last 24h):

Total publishes:     1,247
Successful:          1,189 (95.3%)
Failed:              58 (4.7%)

Failure reasons:
  - Validation:      23 (39.7%)
  - Security scan:   18 (31.0%)
  - Network:         12 (20.7%)
  - Other:           5 (8.6%)

Average duration:    142 seconds
p50:                 89 seconds
p95:                 387 seconds
p99:                 612 seconds

Stage durations (avg):
  1. Validation:     3.2s
  2. Analysis:       8.1s
  3. Image process:  15.4s
  4. Security scan:  45.2s
  5. Validation:     12.3s
  6. Signing:        2.1s
  7. Assembly:       42.8s
  8. Indexing:       4.2s
  9. Finalize:       1.1s
```

### Scanning Statistics

```
Security Scan Results (Last 30 days):

Total scans:         38,419
Critical vulns:      127 (0.3%)
High vulns:          892 (2.3%)
Medium vulns:        3,214 (8.4%)
Low vulns:           12,458 (32.4%)

Clean scans:         21,728 (56.6%)

Most common CVEs:
  1. CVE-2023-43804 (urllib3)    - 234 occurrences
  2. CVE-2024-0001 (openssl)     - 189 occurrences
  3. CVE-2023-12345 (jq)         - 156 occurrences

Secrets detected:    42 (0.1%)
  - API keys:        28
  - Private keys:    9
  - Passwords:       5
```

## Configuration

### Pipeline Configuration

```yaml
# ~/.astro/publish-config.yaml
publishing:
  # Validation
  validation:
    strict_semver: true
    require_readme: true
    require_changelog: false
    min_readme_length: 100

  # Security
  security:
    require_scan: true
    block_on_critical: true
    block_on_high: false
    max_medium_vulns: 10
    require_signature: false
    detect_secrets: true
    check_licenses: true

  # Performance
  parallel_scans: true
  chunk_size: 5242880  # 5MB
  max_retries: 3
  retry_delay: 1000  # ms

  # Notifications
  notifications:
    webhooks:
      - url: https://hooks.slack.com/services/XXX
        events: [published, failed]
    email: true
```

## Best Practices

### Before Publishing

1. **Test locally**: `astro dev --watch`
2. **Validate**: `astro validate`
3. **Scan**: `astro build --scan`
4. **Sign**: Always sign production artifacts
5. **Document**: Complete README and CHANGELOG

### During Publishing

1. **Use semantic versioning**: 1.2.3, not v1.2.3
2. **Tag appropriately**: Descriptive, searchable tags
3. **Review scan results**: Fix high/critical vulnerabilities
4. **Check quotas**: Ensure sufficient quota
5. **Monitor progress**: Watch for warnings

### After Publishing

1. **Verify**: `astro verify myorg/agent:1.0.0`
2. **Test install**: `astro install myorg/agent:1.0.0 --dry-run`
3. **Update docs**: Link to registry page
4. **Announce**: Share in community channels
5. **Monitor**: Track downloads and feedback

## Future Enhancements

### Advanced Scanning
- **Dependency graph analysis**: Transitive vulnerability detection
- **Behavioral analysis**: Runtime security analysis
- **Custom policies**: Organization-specific rules
- **Compliance scanning**: SOC2, HIPAA, GDPR checks

### Performance
- **Delta uploads**: Only upload changed parts
- **Compression**: Aggressive compression for knowledge bases
- **CDN integration**: Faster global distribution
- **Caching**: Multi-level caching strategy

### AI-Powered Features
- **Auto-tagging**: ML-based tag suggestions
- **Quality scoring**: Automated quality assessment
- **Vulnerability prediction**: Predict future vulnerabilities
- **Usage recommendations**: Suggest similar agents

This pipeline ensures every published agent meets security, quality, and consistency standards while providing a fast and reliable publishing experience.
