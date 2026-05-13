# Fix `ast project` failing on `.env` load

## Summary

Running `ast project` (the parent command, equivalent to `ast project start`) failed with `failed to read .env file: read <workingDir>: is a directory`. The `--env` flag was only registered on the `start` subcommand, so when the parent command ran the same handler it received an empty `envFile`. `filepath.Join(workingDir, "")` then resolved to the working directory itself, which `os.Stat` accepted and `godotenv.Read` rejected.

## Design

The flag-registration loop in `cmd/dev.go` already had a comment indicating both `devCmd` and `devStartCmd` should receive the shared flags, but the slice only contained `devStartCmd`. Adding `devCmd` brings the registration in line with the comment and with the fact that both commands share `runDevStart` as their handler.

## Migration

None. Existing `ast project start` invocations are unchanged.
