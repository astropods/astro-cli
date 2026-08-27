# Wiring an HTTP route in astro-server

Nearly every `/api/v1` route in astro-server registers through one builder:
`apps/astro-server/internal/openapi/spec.go`. This guide covers how to add a
route the idiomatic way, how auth middleware attaches to a route group, how
path and query parameters get typed, and what the one automated wiring check
actually verifies.

## What the builder does, precisely

`internal/openapi.Spec` (imported in `main.go` as `oapispec`) wraps a gin
`RouterGroup`. Its `GET`/`POST`/`PUT`/`PATCH`/`DELETE` methods do two things
in one call:

1. Register the handler on the gin group, exactly like calling
   `group.GET(path, handler)` directly.
2. Record the method, full path, and any documentation options into an
   in-memory `openapi3.T` document.

`main.go` serves that document live at `GET /openapi.json`
(`router.GET("/openapi.json", api.JSON())`, `apps/astro-server/main.go:1120`).
**This produces a real OpenAPI 3.0.3 artifact**, generated from the actual
registered routes on every server start, not a checked-in or hand-authored
spec file. `apps/astro-server/internal/openapi/spec_test.go` confirms the
document is valid JSON with the expected shape (path param conversion, tags,
security, request/response schemas). There's no separate generation script;
the spec only exists as the live `/openapi.json` response.

The trade-off: the spec is only as complete as what routes pass through the
builder. Any route registered directly on a gin group (`group.GET(...)`
instead of `api.GET(group, ...)`) still works, but it's invisible to
`/openapi.json`. See [Escape hatches](#escape-hatches-routes-that-skip-the-builder).

## Adding a new route

Follow the shape used throughout `main.go`'s `setupRoutes`: build (or reuse) a
gin `RouterGroup` with its middleware chain, then register through `api`
instead of the group directly.

```go
// Group scoped to :account, with the middleware chain every account-scoped
// mutation needs: resolve the account, then check the caller's permission.
accountSettings := protected.Group("/accounts/:account")
accountSettings.Use(middleware.ResolveAccount(accountStore))
accountSettings.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))

api.PATCH(accountSettings, "", "Update account", handlers.UpdateAccount(log, accountStore, auditStore),
    oapispec.Tags("Accounts"),
    oapispec.BearerAuth(),
    oapispec.PathParam("account", "Account name"),
    oapispec.Body(&handlers.UpdateAccountRequest{}),
    oapispec.Response(200, &handlers.MessageResponse{}),
    oapispec.Response(400, &handlers.ErrorResponse{}),
)
```

Key points:

- Pass the `*gin.RouterGroup`, not the path alone; the builder reads
  `group.BasePath()` to build the full documented path.
- `summary` (the third argument) is required and short; put detail in
  `oapispec.Desc(...)` if needed.
- Add `oapispec.Tags(...)` so the route groups sensibly in the spec (existing
  tags: `Accounts`, `Agents`, `Billing`, `Avatars`, `Profile`, `Health`, and
  others; reuse one instead of inventing a near-duplicate).
- Add `oapispec.BearerAuth()` on every route that actually requires auth.
  Nothing enforces this automatically; it's a documentation-only flag, and
  omitting it doesn't remove the real `RequireAuth()`/permission middleware.
- Declare the request body with `oapispec.Body(&SomeRequest{})` and each
  possible response with `oapispec.Response(status, &SomeResponse{})` (or
  `oapispec.Response(status, nil)` for a body-less response like `204`). Both
  use reflection (`openapi3gen`) to derive a JSON schema from the Go struct,
  so keep request/response types as real structs with `json` tags rather than
  `gin.H{}` where you can (a few existing routes still return `gin.H{...}`
  from `Response(...)`, which the generator schemas but is less precise).

## Middleware composition

Route groups compose middleware in a fixed, meaningful order. The account
middleware in `internal/middleware/account.go` documents this: each one
"must be used after `ResolveAccount` and `RequireAuth`."

- `authMw.RequireAuth()` establishes the authenticated user and session.
  Applied once, high up, to a shared `protected` group most account/agent
  routes descend from.
- `middleware.ResolveAccount(accountStore)` reads the `:account` path
  param, loads the account, and sets it on the context. Required before any
  of the checks below.
- `middleware.RequireAccountPermission(accountStore, "org:manage")` checks,
  for organization accounts, the permission against the WorkOS JWT's
  permissions claim (the session must be scoped to that org, via
  switch-org); for personal accounts, any member has every permission.
- `middleware.RequireAccountMember(accountStore)` is a looser check: the
  caller must simply be a member of the resolved account, no specific
  permission required.
- `middleware.RequireAccountOwner(accountStore)` is the strictest: the caller
  must be the account's recorded `owner_user_id`, not just an org admin.
  Used for destructive actions like account deletion, since ownership is a
  distinct, narrower concept than the `org:manage` permission.

A typical chain, in order:

```go
group := protected.Group("/accounts/:account")
group.Use(middleware.ResolveAccount(accountStore))
group.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
```

Deploy-token-authenticated routes (called by messaging containers, not
end users) use a separate, single middleware instead:
`middleware.RequireDeployToken(cfg.Security.DeployTokenSecret)`.

### Deployment routes: a second wrapper on top of the builder

Routes under `/deployments/:id` go through a further wrapper,
`middleware.DeploymentRoutes` (`internal/middleware/deployment_routes.go`),
constructed once with `middleware.NewDeploymentRoutes(api, protected, deploymentRouteCatalog)`.
It has methods like `ObservedGET`, `DeferredPUT`, `ModelDeferredPOST`, and
`DataPlaneGET` instead of bare `GET`/`POST`. Each one both calls the
underlying `api.GET`/`api.POST` (so the route still lands in the OpenAPI
spec) and records an authorization classification (an `authz.Action`, or a
"data plane" / "model deferred" marker) in a `DeploymentRouteCatalog`:

```go
deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/stop",
    "Stop a running deployment", handlers.StopDeployment(...),
    oapispec.Tags("Deployments"), oapispec.BearerAuth(), oapispec.PathParam("id", "Deployment ID"),
    oapispec.Response(200, &handlers.MessageResponse{}),
)
```

At the end of `setupRoutes`, `main.go` calls
`deploymentRouteCatalog.Validate(router.Routes())` and exits the process
(`os.Exit(1)`) if validation fails. This is a real startup-time consistency
check, but its scope is narrower than "every route is wired correctly": it
only checks that every live route matching `/deployments/:id` has exactly
one registered authorization classification, and that every registered
classification corresponds to a route that actually exists. Forgetting to
classify a new deployment route (by using bare `protected.GET(...)` instead
of `deploymentRoutes.DeferredGET(...)`, for example) crashes the server on
boot rather than silently shipping an unauthorized route.

## Path and query parameters

- **Path params** are detected automatically from gin's `:name` syntax in
  the path (via a regex over the full path). A detected param is documented
  even if you don't call `oapispec.PathParam`, but without a description.
  Call `oapispec.PathParam(name, description)` to add one; it's otherwise
  identical to the auto-detected entry (both are marked `required: true`).
- **Query params** are never auto-detected. Add one explicitly per param:
  `oapispec.QueryParam(name, description, required)`.
- Both path and query params are documented with a plain string schema
  (`openapi3.NewStringSchema()`) regardless of the value's real type. A
  numeric path param like `/avatar/preset/:index` is still typed as a string
  in the spec; the handler is responsible for parsing/validating it (e.g.,
  `strconv.Atoi`). The builder doesn't infer int/bool/enum types for
  parameters, only for request/response bodies.

## Escape hatches: routes that skip the builder

A handful of routes in `main.go` register directly on a gin group instead of
through `api`, and are legitimately absent from `/openapi.json`:

- Root-level infra: `/openapi.json` itself, `/livez`, `/readyz`, `/healthz`,
  `/schema/package.json`, and the local-dev `/assets` static mount. Comments
  in `main.go` mark these as intentionally outside the API spec.
- `/auth/*` (login, callback, logout, refresh, switch-org): a separate
  top-level group, not versioned under `/api/v1`.
- Webhook receivers (`/webhooks/metronome`, `/webhooks/stripe`) and the CLI
  install/download routes: infra endpoints outside the versioned API.
- One documented exception inside `/api/v1`:
  `GET /accounts/:account/billing/invoices/:invoiceId/pdf` registers
  directly on its group with a comment explaining why (it streams
  `application/pdf`, not JSON, so a `Response(...)` schema doesn't fit).

One gap that isn't commented: the deploy-token-authenticated routes
(`GET /api/v1/deployments/authorize` and
`POST /api/v1/deployments/feedback/scores`, both under `deployTokenRoutes`
in `main.go`) register directly on their gin group rather than through
`api`. They're real `/api/v1` endpoints but don't appear in
`/openapi.json`, and nothing marks this as deliberate the way the PDF route
does.

## The wiring test, and what it actually checks

`apps/astro-server/handlers/route_wiring_test.go` (`TestRoutePermissionWiring`
and `TestAccountDeleteAsksTheOwnerColumn`) is worth understanding precisely,
because it's easy to over-trust:

- It does **not** import or exercise `main.go`'s real `setupRoutes`. `main.go`
  is `package main`, so nothing outside it can import that function. Instead,
  the test hand-builds a second, miniature gin router inside the `handlers`
  test package, registering the same paths and the same middleware calls
  (`middleware.ResolveAccount`, `middleware.RequireAccountPermission`, etc.)
  that `main.go` uses for that route.
- It then fires real requests through that miniature router with different
  JWT permission sets and session org scopes, and asserts the expected
  200/403.
- What it actually verifies: that a given middleware combination
  (`ResolveAccount` + `RequireAccountPermission(..., "org:manage")`, for
  example) enforces the permission and org-scoping rules correctly, and that
  `RequireAccountOwner` checks `accounts.owner_user_id` rather than the
  session's role claim.
- What it does **not** verify: that `main.go` actually wires each real route
  with the middleware this test assumes. The test's own comment says it
  matches "the setup in main.go's setupRoutes," but nothing enforces that
  match automatically. If a route's middleware changes in `main.go` and the
  test isn't updated to match, the test keeps passing against its own
  (now stale) copy of the wiring.
- Its coverage is also partial: it covers the permission chain for account
  settings, invitations, members, quota-increase, agent visibility, and the
  variables vault. It does not cover most of the ~190 routes wired through
  `api`/`deploymentRoutes` in `main.go`.

Keep it green because it's the only executable check on permission-middleware
behavior for the routes it covers, but when you change a route's middleware
in `main.go`, update the matching case here by hand; nothing else will catch
a mismatch.

## Test coverage of the builder itself

`internal/openapi/spec_test.go` is the only test file in the `openapi`
package. It builds a small router with a handful of example routes and
asserts the generated JSON has the right paths, path-param conversion, tags,
security, and query params. It doesn't touch anything in `main.go`; nothing
fails if a real route in `main.go` forgets a `Tags(...)` call, an expected
`Response(...)`, or bypasses the builder entirely.
