# Messaging Ingress OIDC Authentication via WorkOS

## Summary

The messaging web adapter ingress had no authentication in front of it. This adds WorkOS OIDC authentication at the AWS ALB layer, with no changes to the container.

## Design

Authentication is handled entirely by the ALB's built-in `authenticate-oidc` action. When a browser hits the messaging ingress unauthenticated, ALB redirects to WorkOS (AuthKit), validates the response, sets a session cookie, and forwards the request with identity headers (`x-amzn-oidc-identity`, `x-amzn-oidc-data`). The container sees only authenticated requests.

ALB fetches the WorkOS client credentials directly from AWS Secrets Manager using the ALB controller's IRSA role. The secret must contain `{"clientId":"...","clientSecret":"..."}`.

OIDC is enabled by setting `MESSAGING_OIDC_ISSUER` on the server. If the env var is absent, no auth annotations are added and ingresses behave as before. All five vars must be set for auth to activate:

```
MESSAGING_OIDC_ISSUER=https://<authkit-subdomain>.authkit.app
MESSAGING_OIDC_AUTH_ENDPOINT=https://<authkit-subdomain>.authkit.app/oauth2/authorize
MESSAGING_OIDC_TOKEN_ENDPOINT=https://<authkit-subdomain>.authkit.app/oauth2/token
MESSAGING_OIDC_USERINFO_ENDPOINT=https://<authkit-subdomain>.authkit.app/oauth2/userinfo
MESSAGING_OIDC_SECRET_ARN=<arn from Terraform>
```

The WorkOS redirect URI registered in the OIDC application must be `https://*.<ingress-domain>/oauth2/idpresponse` (ALB's fixed callback path).

Infra: a new `aws_secretsmanager_secret` and IAM policy attachment are required — see `docs/01-spec/messaging-oidc-auth-spec.md`.

## Migration

No action required for existing deployments. OIDC auth activates on next redeploy of an agent once the env vars are set on the server.
