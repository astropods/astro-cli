# Go testing conventions in astro-server

`apps/astro-server` has 377 `_test.go` files and no single documented
convention for how to write one. This guide describes what the existing
tests actually do, not an idealized process. Where practice is inconsistent,
it says so instead of picking a winner.

## The three shapes of test

Almost every test in this codebase is one of three kinds:

1. **Handler test**: spins up a real `gin.Engine`, wires a handler function
   onto a route, and sends an `httptest` request through it. The DB is
   mocked with `sqlmock`; external HTTP calls are mocked with `httptest.Server`.
2. **Store test (sqlmock)**: calls a `Store` method directly against a
   `sqlmock`-backed `*sql.DB` and asserts on the query, args, and result,
   with no handler or HTTP layer involved.
3. **Integration test (real Postgres)**: calls a `Store` or handler against
   an actual Postgres database (`DATABASE_URL`), for behavior a mock can't
   verify honestly (real constraints, triggers, transaction semantics,
   multi-row SQL correctness).

Pure-unit tests of plain functions (parsers, validators, formatters) show up
throughout all three files above and don't need their own category. They're
just a `func` and a table of inputs and outputs, with no DB or HTTP involved.

## 1. Handler tests

Handler tests build a minimal `gin.Engine`, register the handler under test
on a route, and drive it with `httptest.NewRequest` and `httptest.NewRecorder`.
Example shape (`handlers/supabase_test.go`):

```go
router := gin.New()
router.Use(injectTestSession())
router.GET("/api/v1/accounts/:account/supabase/status", SupabaseStatus(log, pipes.New("fake-key")))

req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/status", nil)
rec := httptest.NewRecorder()
router.ServeHTTP(rec, req)
```

### The shared test harness

`handlers` is one Go package, so every `_test.go` file in it compiles
together: a helper defined in one file is usable, unqualified, from any
other test file in the package. The harness lives in
`handlers/github_account_test.go`, not in a dedicated `testutil_test.go`
(there isn't one):

- **`injectTestSession()`** (`github_account_test.go`): a `gin.HandlerFunc`
  that sets a fake `*auth.Session` on the context, standing in for the real
  `RequireAuth` middleware. Use it as `router.Use(injectTestSession())` when
  the handler needs an authenticated user but not necessarily a resolved
  account.
- **`injectTestAccount()`** (`agents_test.go`): same idea for
  `*account.Account`, standing in for `ResolveAccount`. Stack both when the
  handler needs an authenticated user and a resolved account:
  `router.Use(injectTestSession(), injectTestAccount())`.
- **`rewriteTransport`** (`github_account_test.go`): an `http.RoundTripper`
  that rewrites every outbound request's scheme and host to point at a local
  `httptest.Server`, installed via `http.DefaultTransport = &rewriteTransport{server: srv}`
  (saved and restored after the test). Use this when the handler under test
  makes outbound HTTP calls (WorkOS, Supabase, GitHub, and so on) through
  whatever client uses `http.DefaultTransport`, and you want to serve those
  calls from a fake server instead of hitting the network. `supabase_test.go`'s
  `installSupabaseStub` and `github_account_test.go`'s `installExternalAPIStub`
  are both examples of wrapping this into a one-call setup/teardown helper
  that returns a `func()` to restore the old transport.

To write a new handler test that needs this harness, put it in any
`handlers/*_test.go` file, build a `gin.New()` router, add
`injectTestSession()`/`injectTestAccount()` as needed, and if the handler
calls out over HTTP, install a `rewriteTransport`-backed stub server before
constructing the router. Don't re-implement any of these three: they're
already package-visible.

Handler tests almost always mock the DB with `sqlmock` rather than hitting
Postgres (see below); a handler test that needs Postgres-only behavior is
rare and would be written as an integration test instead (see the systemaudit
example there).

## 2. Store tests (sqlmock)

A `Store` test constructs a `sqlmock.New()` pair, wraps it in the package's
`NewStore(db)`, sets expectations on the exact query and args, and calls the
method. Example (`internal/auditlog/store_test.go`):

```go
db, mock, err := sqlmock.New()
mock.ExpectQuery("SELECT resource_id, actor_id FROM audit_logs").
    WithArgs("acct-1", AgentRegister, "agent", "agent-a", "agent-b").
    WillReturnRows(sqlmock.NewRows([]string{"resource_id", "actor_id"}).
        AddRow("agent-a", "user-1").
        AddRow("agent-a", "user-2"))

s := NewStore(db)
result, err := s.BulkDistinctActorsFor(context.Background(), "acct-1", AgentRegister, "agent", []string{"agent-a", "agent-b"})
```

What this does verify:
- The method sends (approximately) the SQL it's expected to, with the
  arguments the caller passed in, in the order given.
- The method correctly maps the fake rows `sqlmock` returns back into Go
  values (the scan/mapping logic is real and exercised).
- Error paths: a test can make `sqlmock` return an error from a query and
  check the method surfaces it correctly.
- Some tests explicitly call `mock.ExpectationsWereMet()` at the end to
  assert every expected query actually ran; not all tests do this (see
  below).

What this does **not** verify:
- That the SQL is syntactically valid, or that it would actually behave as
  intended against real Postgres (correct join semantics, actual constraint
  behavior, real transaction isolation). `sqlmock` matches the query string
  you told it to expect (by default a regex/substring match, not a real
  parser) and returns exactly the rows you hand it, so it can't tell you the
  query is wrong.
- Anything about concurrent access, locking, or trigger-driven side effects.

`mock.ExpectQuery(...)`'s pattern is a partial match by default, which is why
most tests pattern-match on a distinctive fragment of the query
(`"SELECT resource_id, actor_id FROM audit_logs"`) rather than the whole
SQL string. This keeps tests from breaking on incidental formatting changes,
at the cost of not catching every SQL regression a stricter match would.

**Inconsistency worth noting:** not every sqlmock test calls
`mock.ExpectationsWereMet()`. Several do (as in the auditlog example above);
several don't. A test that skips this check will still pass if the code
under test never issues the expected query at all: it only catches "query
ran with different args," not "query didn't run."

## 3. Integration tests (real Postgres)

These exist because a sqlmock test can't tell you a query is syntactically
valid, that a constraint fires, or that a multi-statement transaction
actually behaves as intended against real Postgres. Write one instead of a
sqlmock test when the behavior you're testing genuinely depends on Postgres
doing something a mock can't fake honestly: unique/foreign-key constraint
enforcement, `ON CONFLICT` semantics, row-level locking, trigger-driven
columns, or SQL whose correctness (not just "was this method called") is the
point of the test. `internal/systemaudit/store_integration_test.go`'s
`TestChecksAreValidSQL` runs every registered check's raw SQL against a real
database specifically because a mock would let a broken query pass silently.

**Two different patterns coexist in the codebase for gating on
Postgres, and there's no written rule for choosing between them:**

**Pattern A, build-tagged, hard-fails without a database.**
Files start with `//go:build integration` and their `testDB(t)` helper calls
`t.Fatal("DATABASE_URL must be set for integration tests")` if the env var
is empty. These are excluded from a plain `go test ./...` entirely (the
build tag means the file isn't even compiled) and only run when tests are
built with `-tags integration`. Examples: `internal/systemaudit/store_integration_test.go`,
`internal/account/gateway_budget_integration_test.go`,
`internal/riverqueue/undeploy_integration_test.go`, and most of `e2e/`
(a handful of files there are `//go:build k8s` instead, or `integration ||
k8s`, requiring a live cluster rather than just a database — check a file's
own build tag before assuming `-tags integration` is enough to run it).

**Pattern B, no build tag, skips gracefully without a database.**
Files have no build tag (they're ordinary package test files) and check
`os.Getenv("DATABASE_URL")` themselves, calling
`t.Skip("DATABASE_URL not set, skipping integration test")` (or an
equivalent message) if it's empty, instead of failing. The exact helper
isn't a single shared name: some packages have their own `testDB(t)`
(`internal/deploymentstore/store_test.go`,
`internal/knowledgestore/store_test.go`), `internal/account/bindings_db_test.go`
names its version `bindingsTestDB` to avoid colliding with the package's
other test helpers, and `internal/pgnotify/notify_test.go` inlines the
check with no separate helper at all. `internal/leaderelection/advisory_test.go`
follows the same skip-not-fail shape. These compile and run as part of the
default `go test ./...`/`moon run astro-server:test` suite; without
`DATABASE_URL` set they report as skipped, not failed, and with it set they
actually run.

The practical difference: Pattern A tests never run at all in the default
suite (CI's ordinary "Test Go applications" job never sees them); Pattern B
tests are always compiled and silently skip unless the environment happens
to provide `DATABASE_URL`. Neither is documented as the preferred approach,
and which one a given file uses looks like it depends on whoever wrote it,
not on a rule. If you're adding a new Postgres-backed test, matching the
convention of neighboring files in the same package is the closest thing to
a guideline that currently exists.

### Running integration tests

- `moon run astro-server:test-integration`: mirrors CI's `test-go-integration`
  job. Runs `scripts/e2e.sh integration`, which stands up a local Postgres,
  applies schema migrations, sets `DATABASE_URL`, and runs
  `go test -tags integration ./e2e/... ./internal/...`. This is what actually
  compiles and runs Pattern A files; it does not require Kubernetes.
- `moon run astro-server:e2e`: the broader suite requiring both Postgres
  and a Kubernetes cluster (`vcluster`/`kind`), needed for tests that
  exercise real deployment/K8s behavior, not just Postgres.
- `moon run astro-server:test-billing`: a separate, bespoke billing
  coverage script (`scripts/test-billing.sh`). Runs the billing-owned and
  shared packages unconditionally, and folds in the Postgres-backed
  `internal/account` integration tests only if `DATABASE_URL` is set; without
  it, the run still passes but reports itself `PARTIAL` rather than silently
  looking complete. Setting `BILLING_REQUIRE_DB=1` turns a missing
  `DATABASE_URL` into a hard failure instead of a partial pass.
- Plain `moon run astro-server:test` (`gotestsum ... ./...`) compiles and
  runs everything except Pattern A (build-tagged) files. Pattern B files
  run too, but skip any sub-test whose `testDB(t)` finds no `DATABASE_URL`.

To run integration tests locally against your own Postgres instead of the
script's throwaway one, set `DATABASE_URL` to a Postgres connection string
with the schema already migrated, then run `go test -tags integration ./...`
from `apps/astro-server` (add `./e2e/...` explicitly if you want that
directory included).

## 4. Table-driven tests

Table-driven tests (`tests := []struct{...}{...}` iterated with `t.Run`) are
the dominant style for anything with more than one or two input/output
cases: about a third of all test files in the codebase use this shape, and
it's the default choice for pure-function and validation tests in
particular. Example (`handlers/supabase_test.go`):

```go
tests := []struct {
    name     string
    body     string
    wantHost string
    wantUser string
}{
    {"object shape", objectBody, "aws-1-us-east-1.pooler.supabase.com", "postgres.abcdef1234567890"},
    {"array shape", arrayBody, "aws-0-eu-west-2.pooler.supabase.com", "postgres.zzz9999888877776"},
    {
        "prefers primary over replica",
        `[{"database_type":"READ_REPLICA",...}]`,
        "primary.pooler.supabase.com",
        "postgres.primary0000000",
    },
}

for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) {
        cfg, err := supabaseFetchPoolerConfigFromURL(context.Background(), "tok", srv.URL, "ref123")
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if cfg.DBHost != tc.wantHost || cfg.DBUser != tc.wantUser {
            t.Errorf("got host=%q user=%q, want host=%q user=%q", cfg.DBHost, cfg.DBUser, tc.wantHost, tc.wantUser)
        }
    })
}
```

The loop variable is named `tc` or `tt` interchangeably across the codebase
(both are common; neither is preferred), and the struct's discriminating
field is almost always named `name` or left first in the struct so
`t.Run(tc.name, ...)` reads naturally.

**The other common shape, used just as often for handler and scenario
tests, is one `Test` function per scenario instead of one table with
subtests.** `handlers/account_secrets_test.go` has
`TestCreateAccountVariable_SingleEntry`, `TestCreateAccountVariable_BulkEntries`,
`TestCreateAccountVariable_InvalidName`, `TestCreateAccountVariable_MixedCaseNames`,
and so on as separate top-level functions rather than table rows, because
each scenario needs distinct sqlmock expectations or router setup that
doesn't compress cleanly into a shared table. Both shapes are normal; which
one a file uses depends on whether the scenarios share enough setup to make
a table worthwhile.

## 5. Naming conventions

**What's consistent:**
- Every test function starts with `Test` followed by the subject, per Go's
  own requirement (`go test` only picks up `func TestXxx(t *testing.T)`).
- Table-driven subtests are named after what varies, in plain lowercase
  phrases (`"object shape"`, `"prefers primary over replica"`,
  `"single line"`), not a repeat of the parent test's name.
- Helper functions used only by tests live in `_test.go` files and are
  lowercase and unexported (`testDB`, `setupAgentTestRouter`,
  `injectTestSession`), same as any other unexported Go helper.
- `t.Helper()` is called at the top of shared test helpers that call
  `t.Fatal`/`t.Skip` on the caller's behalf (`testDB`, `ensureTestAccount`),
  so failures report at the call site, not inside the helper.

**What varies:**
- `TestXxx_Yyy` (`TestCreateAccountVariable_InvalidName`) and one long
  descriptive `TestXxxYyyZzz` with no underscore
  (`TestAccessGroupDisabledNeverCallsWorkOS`) are both common: roughly two
  thirds of test functions use the underscore form, and the rest read as a
  full sentence in CamelCase. Neither is enforced; both appear side by side
  in the same file in places.
- Whether a sqlmock test calls `mock.ExpectationsWereMet()` (see above).
- Whether a Postgres-gated test uses the build-tag-and-fail pattern or the
  no-tag-and-skip pattern (see above): this is the least consistent part of
  the whole convention.

## What this guide doesn't prescribe

There's no written rule anywhere in the codebase for when a test should be
sqlmock-based, a real-Postgres integration test, or a pure-unit test with no
DB at all. That choice is made per-package by whoever wrote the test, and
this guide describes the resulting practice rather than inventing a rule
that isn't actually followed. In practice: pure functions (parsers,
validators) get plain table-driven unit tests; most `Store` and handler
logic gets sqlmock because it's fast and needs no infrastructure;
Postgres-specific behavior (constraints, real SQL correctness, transactions)
gets an integration test, in whichever of the two gating styles above the
surrounding package already uses.

Some packages have no test files at all. This guide doesn't re-litigate
that; see the relevant `docs/README.md` area-map rows' Verify columns
(`avatar`, `accountvars`, `blueprintcache`, `knowledgecache` are called out
there as zero-coverage as of the last check).
