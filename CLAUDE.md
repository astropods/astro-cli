# astro-cli conventions

This repo is normally checked out as `astropods/astro`'s `modules/astro-cli`
submodule. That repo's `agents.md` (Writing style, Development Workflow)
applies here too; where this file says something different, this file wins.

Keep the CLI's command surface and help text accurate when you change commands,
flags, or default behavior.

## Testing

Use `github.com/stretchr/testify` for all Go tests — `require` for fatal assertions, `assert` for non-fatal ones. Do not use `t.Fatal` / `t.Error` / `t.Errorf` directly.

Prefer table-driven tests with `t.Run` subtests. Only write fine-grained individual test functions when the setup or behavior is meaningfully different from other cases.

### Credentials in tests

- Use `writeAccountTestCredentials(t, creds)` (defined in `account_test.go`) to write a credentials file. Always call `t.Setenv("HOME", t.TempDir())` first so the file lands in a temp dir.
- Use `accountTestCreds(currentAccount)` for a standard profile — the argument sets the active `CurrentAccount`; the profile always includes personal ("alice") and two org accounts. Pass a custom `*auth.Credentials` only when you need an account name or structure that doesn't match this standard set.
- Never call `t.Setenv(auth.EnvAccessToken, ...)` in `cmd` package tests. `auth.GetEnvAccessToken()` uses `sync.Once` — setting the env var in one test permanently caches the value for the entire test binary, bypassing auth checks in later tests.

### What "tested" has to cover

A green suite is not evidence the change is wired up. Audit each behavior the
change introduces and check that something fails if it regresses:

- **Flag registration.** A flag switched to `Flags().Var(...)` can revert to a
  plain `String` with every other test still passing, because nothing asserts
  which value type a flag carries. Assert the default, that `Value.Type()` is
  still `"string"`, and that an invalid value is rejected.
  `TestEnvFileFlagsAreWiredToTheValidatingValue` is the worked example.
- **Anything installed once at startup**, such as `SetFlagErrorFunc` on
  `rootCmd`. Unit-test the function and assert the registration separately;
  the function alone proves nothing about whether it is reachable.
- **The seam the output actually reads from.** `assembleDevEnv` narrates
  through its `OnStage` callback rather than its return value, so a count
  asserted only on the return proves nothing about what the user sees.

### Tests that pass alone and fail in the suite

Several tests mutate package-level commands: `agent_deploy_test.go` calls
`blueprintDeployCmd.ResetFlags()` and re-registers flags by hand. A test
asserting on one of those singletons therefore depends on test order, and the
deploy tests exercise a hand-built flag rather than the registered one.

- Assert against a fresh `&cobra.Command{}` handed to the registrar
  (`registerDeployCommonFlags`, `registerConfigureFlags`) instead of the
  package-level command.
- Never conclude from `go test -run TestOne`. Run the whole package with
  `-count=1`, twice, before believing a result.

### pflag traps

- `Value.Type()` must return `"string"` for any flag read through
  `flagString`. pflag's `GetString` rejects a mismatched type and the read
  silently returns `""`.
- A custom `Value` must accept `""` as "clear the flag". Tests reset shared
  commands with `Flags().Set(name, "")` under `//nolint:errcheck`, so
  rejecting an empty value turns the reset into a silent no-op that leaks
  state into later tests.
- pflag wraps an error from `Value.Set` as `invalid argument "x" for "--f"
  flag: `. Strip it once in `unwrapFlagValueError` (`cmd/root.go`), not per
  call site.

### Reading results honestly

- `golangci-lint` caches per checkout. Export a fresh `GOLANGCI_LINT_CACHE`
  when working in a git worktree, or it replays another tree's findings
  against code that no longer matches.
- Capture the exit code of the command you care about. `go build ./... | head`
  reports `head`'s status, so a failing build reads as a pass.

## Command authoring rules

### Authentication & account resolution

- Always use `getCurrentAccountToken(cmd.Context())` to obtain both the active account name and a scoped API token in one call. Never call `getUserNamespace`, `auth.AddAuthHeader`, or `auth.NewTokenManager` directly in command handlers.
- The returned `AccountToken{Account, Token}` is the only credentials object commands should work with.

### HTTP calls

- Always use `apiCall(ctx, method, url, body, at.Token, verbose, &dest)` for all API requests. Never create `http.Client`, `http.NewRequest`, or manage response bodies manually in handlers.
- `apiCall` returns `(int, error)`. Check specific status codes first (`if status == http.StatusNotFound`), then check `if err != nil`. Never use `strings.Contains(err.Error(), "status 404")`.
- Always read `verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")` and pass it to `apiCall`.

### Output

- All output must go through `w := cmd.OutOrStdout()`. Never use `fmt.Printf`, `fmt.Println`, or write to `os.Stdout` directly in command handlers.
- Pass `w` to color writers (`color.New(...).Fprint(w, ...)`) and tabwriter (`tabwriter.NewWriter(w, ...)`).

### User-facing messages

All user-visible error strings and multi-line status messages belong in `cmd/messages.go`. Do not inline them in command handlers.
- Error-returning functions are named `errXxx`; string-returning functions are named `msgXxx`.
- Tests must assert against the message function directly (exact-string comparison) rather than substring/keyword checks. This keeps copy changes and test expectations in sync automatically.

### Flags

- Register per-command flags with `cmd.Flags().Bool(...)` / `cmd.Flags().GetBool(...)`. Never use shared package-level variables for flags that appear on multiple sibling commands (e.g. `--json`). Package-level flag vars leak state across tests.

### URL construction

- Use `apiPath(baseURL, account, operation, parts...)` for all API paths — it accepts variadic trailing parts so sub-resources like `/archive` or `/visibility` are just additional arguments (e.g. `apiPath(base, account, "agents", name, "archive")`).
- Always expose a package-level `xxxServerURLOverride` var and a `xxxBaseURL()` helper that reads it first, then falls back to `auth.DefaultServerURL`. Use the helper everywhere instead of reading `auth.DefaultServerURL` directly.

### Context

- Always pass `cmd.Context()` — never `context.Background()` — to `apiCall` and `getCurrentAccountToken`. The cobra context carries cancellation and test-injected values.
