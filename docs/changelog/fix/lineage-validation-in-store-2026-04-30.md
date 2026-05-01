# Lineage Validation in `Store` Write Path

## Summary

Every deployment row carries a lineage tuple `(source_account_id, agent_name, build_id)` that names the publishing account, the agent, and the specific build deployed. Downstream readers use this tuple as the lookup key for the in-product upgrade signal, the redeploy form's "deploy from this source" affordance, and the source-account display chip on each deployment card. A row whose tuple does not refer to a real `agent_versions` row points readers at a build (or worse, an account) that doesn't actually publish what the row claims.

The HTTP write path already protects this: `prepareDeployment` calls `agentIndex.GetVersion(sourceAcct.ID, agentName, buildID)` before constructing `SaveDeploymentParams`, and any deploy that doesn't resolve a real version returns 4xx before the Store is touched. This PR moves the same check inside `deploymentstore.Store` so the invariant is owned by the storage boundary instead of the handler. Any future write site that constructs `SaveDeploymentParams` directly — admin tools, scripts, a new endpoint, a backfill — inherits the guard automatically.

## Security

The lineage tuple is read at every render of the deployments table and at every redeploy. A row whose `source_account_id` does not actually publish `agent_name@build_id` causes three concrete failure modes downstream:

1. **Misleading upgrade signal.** The client computes "is there a newer build?" against `(source_account_id, agent_name)`. If the source is wrong, the prompt offers users an "upgrade" to a build the wrong account published — a lineage-spoofing shape: the user thinks they're following the publisher they originally trusted.
2. **Wrong redeploy target.** The redeploy form deep-links to the source account's blueprint page. A misattributed row routes the operator to an unrelated account's build catalog.
3. **Misattribution in the UI.** The deployments table shows the source-account name as the "published by" chip. A wrong tuple silently displays the wrong publisher.

The class of write that produces such rows is "any caller of `SaveDeploymentPending` or `UpdateDeploymentPending` that doesn't go through `prepareDeployment` first." Today there are no such callers in production, so this PR is hardening rather than a hot-fix for an exploitable bug. It closes the class at the storage boundary, which means:

- A future endpoint or admin command that calls the Store directly cannot land a misattributed row.
- A race between `prepareDeployment.GetVersion` and the Store write — for example a publisher's account being deleted (cascading away its `agent_versions`) between the two — is narrowed: the Store re-checks at write time and rejects rather than persisting an immediately-orphaned tuple.
- Tests that exercise the wired `Store` against real Postgres pin the cross-account-drift case directly, so the boundary is verifiable rather than a comment.

The gate fires only when `p.SourceAccountID != ""`. Rows that predate the column or callers that intentionally omit attribution take the legacy path (read-time spec-JSON fallback to the `source.account` name). That path is unaffected by this PR.

## Design

A small interface and a fluent setter; nil validator means no validation.

```go
// internal/deploymentstore/store.go
type LineageValidator interface {
    ValidateLineage(accountID, name, buildID string) error
}

func (s *Store) WithLineageValidator(v LineageValidator) *Store {
    s.validator = v
    return s
}
```

`*agentindex.Index` satisfies `LineageValidator` via a one-line wrapper around its existing `GetVersion`:

```go
// internal/agentindex/index.go
func (idx *Index) ValidateLineage(accountID, name, buildID string) error {
    _, err := idx.GetVersion(accountID, name, buildID)
    return err
}
```

The error-only return is deliberate. The Store only needs to know whether the tuple resolves; it doesn't need to load the version object. Keeping the interface error-only also means `deploymentstore` doesn't import `agentindex` (the wrapper lives on the agentindex side, not the deploymentstore side), so the two packages remain decoupled the way they are today.

`SaveDeploymentPending` and `UpdateDeploymentPending` both call a private `validateLineage(p)` at the top of the function, before `tx.Begin`:

```go
func (s *Store) validateLineage(p SaveDeploymentParams) error {
    if s.validator == nil || p.SourceAccountID == "" {
        return nil
    }
    if err := s.validator.ValidateLineage(p.SourceAccountID, p.AgentName, p.BuildID); err != nil {
        return fmt.Errorf("lineage validation failed for %s/%s@%s: %w",
            p.SourceAccountID, p.AgentName, p.BuildID, err)
    }
    return nil
}
```

Production wires the validator in `main.go` at the construction site:

```go
deploymentStore := deploymentstore.NewStore(db).WithLineageValidator(agentIndex)
```

`agentIndex` is already constructed for the existing handler wiring, so threading it into the Store is a single chained call. Tests that construct `deploymentstore.NewStore(db)` directly leave the validator nil and the gate becomes a no-op — the ~50 existing test call sites are untouched.

### Why fluent setter rather than a required constructor argument

A required argument would have forced every test call site to either pass a validator or import a `NoopLineageValidator{}` type. The fluent shape makes the wire-up an explicit decision in production (`.WithLineageValidator(...)` reads as "opt in") while keeping the test surface unchanged. The trade-off is that a future refactor could drop the wire and silently disable the gate; the integration test below pins this by asserting the wired Store rejects bad tuples, so a missing wire fails CI rather than slipping through.

## Tests

Six cases in [apps/astro-server/e2e/lineage_validation_test.go](apps/astro-server/e2e/lineage_validation_test.go) (`-tags integration`), all running against real Postgres with a real `agentindex.Index` wired into the Store via `WithLineageValidator`. Sqlmock unit tests are skipped: at this layer they would only verify "if the validator returns an error, the Store returns the same error", which restates the implementation rather than testing the boundary.

**Save path (4 cases)**

- `RejectsUnknownTuple` — `SourceAccountID` is set, no `agent_versions` row seeded → rejection wraps the underlying "build not found" with "lineage validation failed". Asserts the deployments table is unchanged afterwards (the gate must fire before `tx.Begin` so a rejected write leaves no phantom row).
- `AcceptsKnownTuple` — seed the row via `agentindex.Register`, deploy succeeds, exactly one row recorded.
- `SkipsValidationWhenSourceAccountIDEmpty` — empty `SourceAccountID` (legacy/ancient path) bypasses the gate. The tuple is deliberately unseeded; this test passing proves the legacy path is preserved.
- `RejectsCrossAccountDrift` — the lineage-spoofing shape: `SourceAccountID = acctA` but the only matching version exists under `acctB` (same agent name, same build ID). Rejected. The same write with `SourceAccountID = acctB` succeeds, confirming the gate rejects the misattribution rather than the build ID outright.

**Update path (2 cases)**

- `RejectsUnknownTuple` — Save with a seeded build, then Update with an unseeded build ID. Rejected. The original `build_id` is still recorded post-rejection (the gate runs before the SQL UPDATE, not after a partial mutation).
- `AcceptsKnownTuple` — both builds seeded, Save then Update succeeds, the redeployed build is what reads back.

The Update cases exist because the validator block is a copy-paste in two functions; testing only Save would let Update regress silently.

## Migration

None. Production wires the validator automatically via `main.go`. Existing tests that construct `deploymentstore.NewStore(db)` without a validator continue to pass — the gate is opt-in and nil-safe.
