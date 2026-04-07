## Summary

Deployments that referenced account-level variables via the `ref` field were being rejected with `required variable has no value`, even though `ref` is a valid substitute for `value` (it resolves at deploy time). The spec parser was missing the `ref` guard that the server-side resolver already had.

## Design

`Variable` has two ways to supply a value at deploy time:

- `value`: a literal string embedded in the spec
- `ref`: the name of an account-level variable to resolve at deploy time

The validation rules are:
- **Rule 12**: a non-optional variable must have either `value` or `ref` — `!optional && value == "" && ref == ""` is an error.
- **Rule 12e** (new): `value` and `ref` are mutually exclusive — setting both is an error. This was already enforced in the server resolver but is now caught earlier at parse time.

Tests cover the full truth table for all 8 combinations of `(optional × value × ref)`.

## Migration

No changes required. Specs using `ref` on required variables will now pass validation instead of being rejected.
