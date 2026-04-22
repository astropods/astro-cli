# astro-cli conventions

## Spec

The CLI command surface is defined in `docs/02-cli/cli-command-tree.md`. Keep this spec up to date whenever commands are added or removed.

## Testing

Use `github.com/stretchr/testify` for all Go tests — `require` for fatal assertions, `assert` for non-fatal ones. Do not use `t.Fatal` / `t.Error` / `t.Errorf` directly.

Prefer table-driven tests with `t.Run` subtests. Only write fine-grained individual test functions when the setup or behaviour is meaningfully different from other cases.

## Command authoring rules

### Authentication & account resolution

- Always use `getCurrentAccountToken(cmd.Context())` to obtain both the active account name and a scoped API token in one call. Never call `getUserNamespace`, `auth.AddAuthHeader`, or `auth.NewTokenManager` directly in command handlers.
- The returned `AccountToken{Account, Token}` is the only credentials object commands should work with.

### HTTP calls

- Always use `apiCall(ctx, method, url, body, at.Token, verbose, &dest)` for all API requests. Never create `http.Client`, `http.NewRequest`, or manage response bodies manually in handlers.
- For non-2xx errors, `apiCall` returns `"server returned status N"`. Map specific status codes to user-friendly messages with `strings.Contains(err.Error(), "status 404")` etc.
- Always read `verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")` and pass it to `apiCall`.

### URL construction

- Use `apiPath(serverURL, account, operation, parts...)` for standard `/api/v1/{operation}/{account}/...` paths.
- For non-standard paths (sub-resources like `/archive`, `/visibility`), build with `strings.TrimSuffix(serverURL, "/") + fmt.Sprintf(...)`.
- Always use a package-level `xxxServerURLOverride` var for test injection; the URL helper reads it first, then falls back to `auth.DefaultServerURL`.

### Context

- Always pass `cmd.Context()` — never `context.Background()` — to `apiCall` and `getCurrentAccountToken`. The cobra context carries cancellation and test-injected values.
