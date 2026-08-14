# Stop an account's own spend warning from gating it

Stacked on the write path. Third step of customer-set spend controls.

## Summary

A warning and a limit are the same Metronome alert type at different numbers, so
both arrive as `alerts.spend_threshold_reached`. Without a way to tell them
apart, setting a warning would suspend the account for crossing the line it asked
to be warned about.

## Design

The webhook payload carries `alert_name`, which the envelope now parses and
`MetronomeWebhookArgs` now carries. `metronomeSignal` takes it and returns no
signal for `astro:spend_warning`.

Both edges are skipped, not just the reached one. Clearing the latch on the
warning's resolved edge would un-gate an account that the limit had stopped,
which is the more expensive direction of the same mistake.

The limit and the org-wide backstop are unaffected: they gate as before, and the
backstop keeps working for an account that sets no limit of its own.

### The warning needs no signal and no column

Its visible state is already served. `GET /billing/spend` returns
`warning.in_alarm` straight from Metronome's per-customer `customer_status`, so
the client can show a crossed warning without anything mirrored here. A signal
that wrote no flag would only add a row to the signal matrix for no behavior.

Notifying on a crossed warning is a separate piece of work, and it waits on the
Novu workflows, which no billing event has yet.

## Migration

None. `alert_name` is absent from every event we handle today, and an absent name
matches nothing, so existing behavior is unchanged.
