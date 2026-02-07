# Registry Testing Runbook

Guide for testing an Astro registry deployment.

## Prerequisites

- `astro` CLI installed
- Docker running (for image operations)
- Access token (if auth enabled)

## Health Checks

### Check Registry API

```bash
# V2 API endpoint
curl -I https://registry.astromode.ai/v2/

# Expected: 200 OK with Docker-Distribution-API-Version header
```

### Check Namespace Endpoint (requires auth)

```bash
curl -H "Authorization: Bearer <token>" https://registry.astromode.ai/api/namespace

# Expected: {"user_id": "...", "organization_id": "..."}
```

## CLI Testing

### Environment Setup

```bash
export ASTRO_REGISTRY_URL=https://registry.astromode.ai
export ASTRO_SERVER_URL=https://astromode.ai
```

### Authentication

```bash
# Login via OAuth device flow
astro login

# Verify authentication
astro whoami
```

### Publish Test

```bash
# Full publish (requires server)
astro publish --tag v1.0.0

# Registry-only test (skip server registration)
astro publish --tag v1.0.0 --skip-register

# Without auth (testing only)
astro publish --tag v1.0.0 --no-auth --skip-register
```

## Minimal Test Agent

Create `astro.yml`:

```yaml
name: test-agent
version: 0.1.0
container:
  image: alpine:latest
```

Run:

```bash
astro publish --tag test --skip-register
```

Image pushes to: `registry.astromode.ai/<namespace>/test-agent:test`

## Troubleshooting

| Issue              | Check                                 |
| ------------------ | ------------------------------------- |
| 401 Unauthorized   | Token expired, run `astro login`      |
| Connection refused | Registry URL correct, service running |
| Push fails         | Docker daemon running, image exists   |
| Namespace error    | Token has valid user claims           |

## Environment Variables

| Variable             | Description                   |
| -------------------- | ----------------------------- |
| `ASTRO_REGISTRY_URL` | Registry endpoint             |
| `ASTRO_SERVER_URL`   | Server for agent registration |
| `ASTRO_ACCESS_TOKEN` | Direct token auth (CI/CD)     |
