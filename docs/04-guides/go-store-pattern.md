# The Store + sentinel-error pattern (Go)

`apps/astro-server/internal/` has around 30 packages that persist state in
Postgres. Almost all of them follow the same shape: a concrete `Store`
struct wrapping `*sql.DB`, package-level sentinel errors for the outcomes a
caller needs to branch on, and `fmt.Errorf("%w", ...)` wrapping everywhere
else. This guide describes that shape so a new package can follow it
instead of inventing its own.

Examples below are drawn from `internal/clusterstore`, `internal/evalitemstore`,
`internal/judgmentstore`, `internal/deploymentstore`, `internal/accountvars`,
and `internal/quota`.

## Interface shape

A `Store` is a concrete struct, not an interface:

```go
// internal/accountvars/store.go
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}
```

This is the default across the codebase (`clusterstore.Store`,
`evalitemstore.Store`, `judgmentstore.Store`, `deploymentstore.Store`,
`auditlog.Store`, and most others). Tests don't mock the struct; they mock
the SQL underneath it with `github.com/DATA-DOG/go-sqlmock`, then construct
a real `*Store` around the mock connection:

```go
// internal/evalitemstore/store_test.go
func newStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db), mock
}
```

A `Store` type is not always literally named `Store`. `internal/quota`
calls its concrete type `DBChecker` because the package is about enforcing
limits, not persisting a domain object; it still wraps `*sql.DB` and
follows the rest of this convention.

**When a package does define an interface**, it's a narrow one written for
a *consumer* that needs to accept either the real store or a hand-written
fake, not a mock of the whole `Store`. Three examples:

- `quota.Checker` / `quota.Reporter` are the interfaces handlers depend on
  (`quotaCheck quota.Checker` in `handlers/deploy.go`), so a handler test
  can pass a `fakeReporter` instead of standing up a `DBChecker`:

  ```go
  // handlers/usage_test.go
  type fakeReporter struct {
  	report map[string]quota.ResourceUsage
  	err    error
  }

  func (f *fakeReporter) Report(_ context.Context, _ string, _ ...string) (map[string]quota.ResourceUsage, error) {
  	return f.report, f.err
  }
  ```

  `quota.DBChecker` implements both interfaces; production wires the
  concrete type, tests wire the fake.

- `deploymentstore.LineageValidator` is a single-method interface
  `Store` itself depends on, so `deploymentstore` doesn't need to import
  `agentindex` just to call one method on it, and tests can construct a
  `Store` with no validator at all (nil-safe no-op).
- `auditlog.Observer` lets other packages react to a successful audit
  write without `auditlog` depending on them.

These interfaces exist to decouple one specific collaborator, not to make
the `Store` itself swappable. Don't add a `type Store interface` purely so
a caller "could" mock it. Construct the real `Store` against `sqlmock`
instead, per the rest of the codebase.

## Constructor convention

`NewStore(db *sql.DB) *Store` (or `New(db *sql.DB) *Store`, e.g.
`clusterstore.New`) is the standard shape: it takes the shared `*sql.DB`
connection pool and returns a pointer to the concrete struct. There's no
error return, since there's nothing to fail at construction time: the pool
is already open and migrations already ran.

A constructor sometimes returns the struct for chaining an optional
dependency instead of taking it as a parameter, when most callers (tests
especially) don't need that dependency:

```go
// internal/deploymentstore/store.go
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) WithLineageValidator(v LineageValidator) *Store {
	s.validator = v
	return s
}
```

Production wires `NewStore(db).WithLineageValidator(agentIndex)` in
`main.go`; every existing test call site keeps working unmodified.

## Sentinel errors

Declare a sentinel as a package-level `var` with `errors.New`, named
`Err<Outcome>`:

```go
// internal/clusterstore/store.go
var (
	ErrNotFound           = errors.New("cluster not found")
	ErrInUse              = errors.New("cluster is still referenced by accounts or deployments")
	ErrInUseByAccounts    = errors.New("cluster still has accounts pinned to it")
	ErrInUseByDeployments = errors.New("cluster has active deployments")
)
```

```go
// internal/evalitemstore/store.go
var ErrAlreadyAdded = errors.New("trace already in the dataset")

// internal/judgmentstore/store.go
var ErrAlreadyJudged = errors.New("trace already judged")

// internal/deploymentstore/store.go
var ErrDuplicateDisplayName = errors.New("display_name already in use by another active deployment")
```

Callers check with `errors.Is`, never a direct `==` comparison (a wrapped
sentinel wouldn't match `==`):

```go
// handlers/dataset_items.go
if errors.Is(err, evalitemstore.ErrAlreadyAdded) {
	// return 409
}

// internal/admingrpc/clusters.go
if errors.Is(err, clusterstore.ErrNotFound) {
	// ...
}
if errors.Is(err, clusterstore.ErrInUse) ||
	errors.Is(err, clusterstore.ErrInUseByAccounts) ||
	errors.Is(err, clusterstore.ErrInUseByDeployments) {
	// ...
}
```

**There is no shared, reused set of sentinel names across packages.**
Each package declares its own (`clusterstore.ErrNotFound`,
`evalitemstore.ErrAlreadyAdded`, `judgmentstore.ErrAlreadyJudged`,
`deploymentstore.ErrDuplicateDisplayName`, `org.ErrOrganizationNotFound`).
`ErrNotFound` and `ErrAlreadyExists`-shaped names recur because the
situations recur, not because there's a shared errors package to import
from. When you add a sentinel, name it for the outcome in your own
package's terms (`ErrNotFound`, `ErrAlreadyAdded`, `ErrInUse`, ...). Don't
go looking for a common one to reuse, there isn't one.

**A "not found" result is not always a sentinel error.** Many read methods
instead return `(nil, nil)` when the row doesn't exist, treating "absent"
as a normal, expected outcome rather than a failure:

```go
// internal/deploymentstore/store.go
func (s *Store) GetDeploymentByID(id string) (*Deployment, error) {
	d, err := scanDeployment(s.db.QueryRow(`SELECT `+deploymentColumns+`
		FROM deployments WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment by ID: %w", err)
	}
	return d, nil
}
```

`accountvars.Store.Get` and `.GetEncryptionKey` do the same. `clusterstore.Get`
takes the other approach and returns `ErrNotFound` instead of `(nil, nil)`.
Pick based on how the caller needs to branch: a plain lookup that a caller
already treats as optional (`if d == nil { ... }`) fits `(nil, nil)`; an
outcome a caller needs to distinguish from other errors while still
propagating an error return (e.g. mapping to a specific HTTP status) fits a
sentinel. Some methods return a third shape instead of either: a `bool`
alongside the value, e.g. `judgmentstore.SetVerdictAndReasons` returns
`(Verdict, []Reason, found bool, err error)` where `found == false` means
"no judgment row for this trace," again not an error.

## Error wrapping

Wrap every error from a lower layer with `fmt.Errorf("%w", err)` and enough
context to identify the call, using `%w` (not `%v`) so `errors.Is`/`errors.As`
still see through it:

```go
func (s *Store) SaveEncryptionKey(accountID string, encryptedDataKey []byte, kmsKeyARN string) error {
	_, err := s.db.Exec(`INSERT INTO account_encryption_keys ...`, accountID, encryptedDataKey, kmsKeyARN)
	if err != nil {
		return fmt.Errorf("accountvars save encryption key: %w", err)
	}
	return nil
}
```

The prefix is usually the package name plus the operation
(`"accountvars save encryption key: %w"`, `"judgmentstore upsert prediction: %w"`),
mirroring the log-message convention elsewhere in this codebase (name the
failure, don't just say "failed").

Translating a raw driver error into a package sentinel happens right where
the raw error surfaces, before it's wrapped further up:

```go
// internal/clusterstore/store.go
func (s *Store) Get(ctx context.Context, id string) (*Cluster, error) {
	row := s.db.QueryRowContext(ctx, baseSelect+` WHERE id = $1`, id)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return c, nil
}
```

The same package translates a `*pq.Error` foreign-key violation into a
typed sentinel by inspecting the violated constraint name:

```go
func (s *Store) tryDeleteCluster(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = $1`, id)
	if err != nil {
		if pgCode(err) == pgForeignKeyViolation {
			switch pgConstraint(err) {
			case "account_clusters_cluster_id_fkey":
				return ErrInUseByAccounts
			case "deployments_cluster_id_fkey":
				return ErrInUseByDeployments
			default:
				return ErrInUse
			}
		}
		return fmt.Errorf("delete cluster: %w", err)
	}
	// ...
}
```

`deploymentstore` translates a unique-violation the same way, via
`pqErr.Code == "23505"`:

```go
if err != nil {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return nil, ErrDuplicateDisplayName
	}
	return nil, fmt.Errorf("failed to insert deployment: %w", err)
}
```

A third common shape: an upsert-style write with `ON CONFLICT ... DO
NOTHING`, where "0 rows affected" *is* the already-exists signal, no error
from the driver at all:

```go
// internal/evalitemstore/store.go
res, err := tx.ExecContext(ctx, `
	INSERT INTO eval_dataset_items (...)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (eval_dataset_id, trace_id) DO NOTHING
`, ...)
if err != nil {
	return fmt.Errorf("evalitemstore add item: %w", err)
}
affected, err := res.RowsAffected()
if err != nil {
	return fmt.Errorf("evalitemstore add item rows affected: %w", err)
}
if affected == 0 {
	return ErrAlreadyAdded
}
```

`judgmentstore.Insert` and `clusterstore.tryDeleteCluster`'s zero-rows
branch (`ErrNotFound`) use the same `RowsAffected() == 0` check for the
same reason: the query succeeded, but it changed nothing, and the caller
needs a typed reason why.

## Transactions

There's no `WithTx` method or a package-level transaction type. A `Store`
method that needs several statements to commit or fail together opens its
own transaction with `s.db.Begin()` (or `BeginTx(ctx, nil)`), defers a
rollback that's a no-op after a successful commit, and commits at the end:

```go
// internal/evalitemstore/store.go
func (s *Store) Add(ctx context.Context, item Item, outputs []Output) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("evalitemstore add: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// ... two related INSERTs against tx, not s.db ...

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("evalitemstore add: commit: %w", err)
	}
	return nil
}
```

When a caller *outside* the store needs its own writes to land in the same
transaction as a store method's writes (for example: save a deployment row
and enqueue a River job atomically), the store method takes a callback that
runs inside its transaction before commit, rather than exposing the `*sql.Tx`
directly:

```go
// internal/deploymentstore/store.go
// SaveDeploymentPending saves a new deployment with status='pending' and creates revision 1.
// The txFn callback runs in the same transaction, allowing the caller to enqueue a River
// job and save normalized spec data atomically.
func (s *Store) SaveDeploymentPending(p SaveDeploymentParams, txFn func(tx *sql.Tx, deploymentID string) error) (*Deployment, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// ... insert the deployment row, its first revision, its first event ...

	if txFn != nil {
		if err := txFn(tx, d.ID); err != nil {
			return nil, fmt.Errorf("failed to run tx callback: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return &d, nil
}
```

`UpdateStatusWithTx(id, u, txFn func(*sql.Tx) error)` is the same shape for
status transitions. Passing `nil` for `txFn` is always valid and just skips
the extra step. That's why `UpdateStatus` is a one-line wrapper around
`UpdateStatusWithTx(id, u, nil)`.

A method that runs multiple statements but never needs to compose with a
caller's own writes just uses a transaction internally without exposing it
at all, as in the `Add` example above. Reach for the `txFn` callback shape
only when an external caller genuinely needs to join the transaction.

## What not to do (how this can drift)

- **`internal/org/client.go`** is *not* an example of this pattern despite
  living in `internal/` next to the Store packages: it's a thin wrapper
  around the WorkOS SDK (an external API, not Postgres), named `Client`
  rather than `Store`. It does follow the sentinel-error half of the
  convention (`ErrOrganizationNotFound`, classified from a WorkOS HTTP 404
  via `errors.Join`), which shows the sentinel-and-`errors.Is` half of this
  pattern is broader than just DB-backed stores. Don't copy its shape
  wholesale as a "Store," though.
- **`internal/accountvars/store.go`** is a clean example of the basic CRUD
  shape, but its `Delete` method breaks the sentinel convention used
  everywhere else in the same file:

  ```go
  func (s *Store) Delete(accountID, name string) error {
  	result, err := s.db.Exec(`DELETE FROM account_variables WHERE account_id = $1 AND name = $2`, accountID, name)
  	// ...
  	rows, _ := result.RowsAffected()
  	if rows == 0 {
  		return fmt.Errorf("variable %q not found", name)
  	}
  	return nil
  }
  ```

  This is a plain `fmt.Errorf`, not a sentinel: a caller can't
  `errors.Is` it to distinguish "not found" from any other failure, unlike
  `clusterstore.tryDeleteCluster`'s identical zero-rows situation, which
  returns `ErrNotFound`. If you're touching this method, prefer returning
  a sentinel (`ErrNotFound` or similar) here instead of extending the
  string-error version further.
- Beyond this method, `accountvars` also has a known ad hoc invariant: the
  "can't change the `secret` flag without a new value" rule lives only in
  the handler, not enforced by the store. See
  `docs/07-feedback/doc-drift-log.md`'s 2026-08-26 Phase C entry. This
  isn't a Store-pattern issue itself, but it's worth knowing before
  treating this package as a template to copy verbatim.
- Don't add a `type Store interface` just to make a store "mockable."
  Every example in this codebase mocks the SQL layer with `sqlmock`
  instead and constructs the real concrete `Store`. An interface is
  reserved for a specific, narrow collaborator (see `LineageValidator`,
  `Observer`, `quota.Checker`/`Reporter` above), not for the store itself.
