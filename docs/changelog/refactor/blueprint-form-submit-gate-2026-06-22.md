# Blueprint wizard: validate on submit instead of disabling the button

## Summary

The New Agent wizard gated forward progress with a `disabled` prop computed from
form correctness (name length/format/availability on the setup step; source
selection on the source step). Under SSR this disabled state could mis-hydrate,
leaving the control in the wrong state on a fresh page load — most visibly, the
setup step's "Continue" button could be clicked with a blank name, advancing the
wizard and allowing a blank blueprint name through.

## Design

Correctness no longer drives the `disabled` attribute. Validation runs in the
submit handlers, against a single field-keyed error store rather than one state
per field:

- Pure per-step validators (`validateSetup`, `validateSource`) hold the rules and
  return a `Record<field, message>` that is empty when the step is valid. A rule
  lives in exactly one place — the same validators back both the gate and the
  setup step's proactive name hint.
- A shared `gate(fieldErrors)` helper publishes the result into the `errors` map
  and returns whether the step passed; both `handleContinueToSource` and
  `handleCreateOrConfirm` call it, so no gating logic is duplicated between them.
- Errors render from `errors[field]`; editing a field calls `clearError(field)`.

Adding a field to the form is now: add a rule to its step validator and render
`errors[field]` — no new state and no new handler branch.

`disabled` is reserved exclusively for in-flight submission state
(`isCreatingBlueprint`/`isPublishing`). The setup step's "Continue" has no async
work, so it carries no `disabled` at all. Because correctness is enforced in the
handler rather than the rendered attribute, the gate holds regardless of how the
button hydrates — the blank-name path is structurally impossible, not merely
hidden behind a disabled button.

## Migration

None. Behavior is unchanged for valid input; invalid input now reports an inline
reason on submit rather than presenting a disabled button.
