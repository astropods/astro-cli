# Summary

Introduce a centralized authorization seam in astro-server ahead of WorkOS FGA deployment-owner checks. No runtime behavior change — routes and handlers are untouched.

This is the first step in a multi-step effort to restrict per-deployment mutations to org admin/owner or the deployment owner via WorkOS FGA.

# Design

Add `internal/authz` with a small vocabulary (`Action`, `ResourceRef`, `Subject`) and a `Checker` interface. `MembershipChecker` is the first implementation: it mirrors today's membership-only model (any account member may act on a deployment's account). `SubjectFromContext` in middleware adapts gin auth context to `authz.Subject` without pulling gin into the authz package.

Deliberately out of scope for this slice: route wiring, WorkOS FGA client and checks, `Subject.MembershipID`, a concrete deployment→account resolver, deploy-time owner persistence, and any change to messaging grants or client-side permission gating. Those layers land in later steps; this PR only establishes the interface and a membership checker that preserves current behavior.

# Migration

Nothing required. This PR is scaffolding only.
