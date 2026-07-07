# fix: filter file types in the import variables picker

## Summary

The "Import variables" dialog on the deploy page let the OS file picker show every file type, even though only `.env`, `.json`, and `.txt` are supported. Users could pick an unsupported file and only find out after selecting it, when the client-side check rejected it. This closes issue #533.

## Design

The file `<input>` in `ImportVariables.tsx` now sets an `accept` attribute, so the native picker hints at the supported types and greys out the rest. The value lives in a small `ACCEPTED_FILE_TYPES` constant next to the existing `ALLOWED_FILE_PATTERN`, so the supported set is easy to find in one place. Nothing about validation changes: `handleFileChange` still checks the file name against `ALLOWED_FILE_PATTERN`, so the `accept` attribute is only a UX hint, not the enforcement.

## Migration

None. Behavior is unchanged for valid files; the file picker is just easier to use.
