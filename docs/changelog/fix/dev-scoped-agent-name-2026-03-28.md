## Summary

`ast dev` failed with `invalid tag "@postman/my-agent": invalid reference format` when the agent name in `astropods.yml` used the scoped `@org/name` format. Docker rejects `@` and `/` in image reference tags, so any spec with a scoped name broke local dev entirely.

## Design

The root cause was that `BuildProject` in the compose builder used `s.Name` verbatim as the Docker Compose project name. Docker Compose derives container and image names from the project name, so the `@org/` prefix propagated into the image tag.

`ast push` already handled this correctly via a local `parseAgentName` helper that strips the prefix before constructing image tags — but that function was unexported and duplicated logic that the compose builder couldn't share.

The fix consolidates the parsing into a single exported `ParseAgentName(raw string) (account, name string)` function in `internal/utils`. All callers — `push`, `build_runner`, and the compose builder — now use it. The compose builder extracts the bare name at the top of `BuildProject` and uses it for both the project name and network name.

## Migration

No action required. Specs using plain names (e.g. `my-agent`) are unaffected. Specs using scoped names (e.g. `@org/my-agent`) will now work correctly with `ast dev`.
