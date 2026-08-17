# Point the CLI submodule at the typed billing refusal

## Summary

A gated CLI command printed the server's raw JSON and exited 1.

The server refuses a suspended account with a 402 and a structured body naming
the reason and the fix. `astro-cli` gained the code to render that body and to
exit 3 for it, but the submodule pointer still named the commit before it, so no
build from this repo carried the change.

Against preview, a deploy to a suspended account printed this:

```
server returned status 402: {"action":"add_card","code":"BILLING_SUSPENDED",
"details":"This account's free credits are used up. Add a payment method to
continue.","error":"Billing suspended","reason":"credits_exhausted"}
```

The refusal was correct and the account was correctly stopped. Only the
presentation was wrong, and a script could not tell a billing refusal from any
other failure.

## Design

The pointer moves from `754060d` to `4d928c7`. Nothing else changes: no Go
module, no build target, no server behaviour.

A submodule pointer does not follow its remote. The CLI merge landed in
`astropods/astro-cli` and left this repo building the older commit, which is why
the fix appeared merged while every build still dumped JSON.

Exit 3 is reserved for a billing refusal, so automation can branch on it rather
than parse a message. Every other failure keeps the exit code and the text it
had, which is what stops this from being a breaking change for existing scripts.

## Migration

None. Rebuild to pick it up:

```
moon run astro-cli:link           # ast-dev, localhost
moon run astro-cli:link-preview   # ast-preview, astropod.ai
```
