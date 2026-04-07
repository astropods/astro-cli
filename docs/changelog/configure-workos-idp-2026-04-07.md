# Messaging Web Adapter Authentication

## Summary

The messaging web adapter ingress had no authentication. This adds opt-in authentication via WorkOS OIDC enforced at the ALB layer — no container changes required. Each deployment explicitly enables auth through the deployment template, and the deploy form exposes a user-friendly "Enable authentication" toggle.

## Design

Authentication is handled entirely by the ALB's built-in `authenticate-oidc` action. When a browser hits the messaging ingress unauthenticated, ALB redirects to WorkOS (AuthKit), validates the response, sets a session cookie, and forwards the request with identity headers (`x-amzn-oidc-identity`, `x-amzn-oidc-data`). The container sees only authenticated requests.

### Opt-in per deployment

Auth is disabled by default. Each deployment template opts in via `interfaces.auth.web.type`:

```yaml
interfaces:
  auth:
    web:
      type: oidc      # enable — use server OIDC config
      # omit/nil     # disable (default)
      # oidc-custom  # reserved for future per-deployment credentials
```

`DeploymentInterfacesAuth` and `DeploymentWebAuth` are new types in `packages/astro-spec`. The server only applies OIDC annotations when both the deployment opts in AND server credentials are configured.

### Credentials flow

The server reads `MESSAGING_OIDC_CLIENT_ID` and `MESSAGING_OIDC_CLIENT_SECRET` from env vars. On deploy, when auth is enabled, it creates a `messaging-oidc` Kubernetes secret in the agent namespace. The ALB controller reads this secret to configure the ALB listener rule. The ALB controller's ClusterRole must include `secrets` read access.

### Frontend

The web adapter card in the messaging interfaces picker shows an "Enable authentication" toggle. Pre-fills correctly when editing an existing authenticated deployment.

### Server env vars

```
MESSAGING_OIDC_ISSUER
MESSAGING_OIDC_AUTH_ENDPOINT
MESSAGING_OIDC_TOKEN_ENDPOINT
MESSAGING_OIDC_USERINFO_ENDPOINT
MESSAGING_OIDC_CLIENT_ID
MESSAGING_OIDC_CLIENT_SECRET
MESSAGING_OIDC_SESSION_TIMEOUT   # default 3600
```

## Migration

No action required for existing deployments. Auth only activates when both the server env vars are set AND the deployment template has `interfaces.auth.web.type: oidc`. Existing deployments without the `auth` block will not get auth applied on redeploy.
