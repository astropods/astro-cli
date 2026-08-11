# Accept Metronome's signed date header

## Summary

Every Metronome webhook delivery to preview has been rejected with a 401 since
the endpoint was registered on 2026-07-21. No billing signal has ever reached
astro: `river_job` holds zero `webhook.metronome` rows and
`account_billing_status` is empty across the whole preview database. An account
that exhausts its credit is therefore never gated and its workloads are never
stopped.

The cause is a header name. Metronome signs over `X-Metronome-Date`, and sends
`Date` alongside it for backward compatibility. The handler read
`Metronome-Webhook-Date`, which Metronome does not send, so the HMAC was
computed over an empty date and could never match. The secret was correct
throughout.

## Design

Signature verification now reads `X-Metronome-Date` and falls back to `Date`.
Nothing else about the scheme changes: still HMAC-SHA256 over
`date + "\n" + rawBody`, hex-encoded, constant-time compared against
`Metronome-Webhook-Signature`.

The bug survived because the test signed and read the same wrong header, so it
was self-consistent and passed while the endpoint rejected every real delivery.
`TestMetronomeWebhook_ValidSignature` is now table-driven over both header names
Metronome actually sends, and `TestMetronomeWebhook_PrefersXMetronomeDate`
asserts that a delivery carrying both headers verifies against the signed one.
Both fail against the previous handler.

The paired Stripe endpoint is unaffected. It delegates to the stripe-go SDK's
`webhook.ConstructEvent`, so header parsing is not hand-rolled there.

## Migration

None. No configuration or secret changes are required. Metronome retries a
failed delivery for up to 48 hours, so notifications outstanding at deploy time
are accepted once this ships, with no need to re-trigger an alert.
