# astro-cli conventions

Keep the CLI's command surface and help text accurate when you change commands,
flags, or default behavior.

## Testing

Use `github.com/stretchr/testify` for all Go tests — `require` for fatal assertions, `assert` for non-fatal ones. Do not use `t.Fatal` / `t.Error` / `t.Errorf` directly.

Prefer table-driven tests with `t.Run` subtests. Only write fine-grained individual test functions when the setup or behaviour is meaningfully different from other cases.

### Credentials in tests

- Use `writeAccountTestCredentials(t, creds)` (defined in `account_test.go`) to write a credentials file. Always call `t.Setenv("HOME", t.TempDir())` first so the file lands in a temp dir.
- Use `accountTestCreds(currentAccount)` for a standard profile — the argument sets the active `CurrentAccount`; the profile always includes personal ("alice") and two org accounts. Pass a custom `*auth.Credentials` only when you need an account name or structure that doesn't match this standard set.
- Never call `t.Setenv(auth.EnvAccessToken, ...)` in `cmd` package tests. `auth.GetEnvAccessToken()` uses `sync.Once` — setting the env var in one test permanently caches the value for the entire test binary, bypassing auth checks in later tests.

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
