## Summary

New deployments with messaging enabled were generating templates with no `auth.web` block, leaving the deploy form's auth toggle in its default off state. Users had to manually enable OIDC auth every time they deployed a new messaging-enabled agent.

## Design

`GenerateDeploymentTemplate` now sets `auth.web.type: oidc` in the messaging interface block when generating a fresh template:

```go
Auth: &spec.DeploymentInterfacesAuth{
    Web: &spec.DeploymentWebAuth{Type: "oidc"},
},
```

Existing deployments are unaffected — the prefill handler restores `auth` from the stored spec before returning the template to the client, so the generated default is only seen on first deploy.

## Migration

No action required. Existing deployments retain their current auth configuration.
