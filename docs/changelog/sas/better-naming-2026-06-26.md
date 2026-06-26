# Clearer interface naming in the deploy form

## Summary

The deploy/configure form labelled the messaging-adapter section "Chat interface" and listed the browser adapter as "Web". Both names were ambiguous: "Chat interface" overlapped with the per-agent chat surface, and "Web" read as a generic ingress rather than the in-browser chat client. This reworks the messaging interface section so its naming, iconography, and options match how users actually talk about these surfaces, and drops a web-only access option that no longer fits the product.

## Design

Messaging interface section of the deploy form:

- The section `FormSection` title becomes **"Messaging interface"** (was "Chat interface").
- The `web` adapter is now labelled **"Astro Chat"** with the description **"Chat directly in the browser"** (was "Web" / "Browser-based chat interface"). The label names the first-party surface in parallel with "Slack" and ties it to the in-app Chat destination. Its `id` (`web`) is unchanged, so persisted specs and the wire format are unaffected.
- The `web` adapter icon becomes the Astro brand mark (new `AstroMark` SVG component, parallel to the existing `Slack` mark) rendered monochrome so it inherits the list's selected/muted icon color, consistent with the header logo treatment.
- The **Protected** toggle is removed from the web chat adapter. Astro Chat lives behind the app's auth, so the open (no-OIDC) cohort option does not apply; web access is governed entirely by its access grants. The form no longer reads or emits `auth.web.public`, and all of the supporting `webPublic` state has been removed. Slack and the custom interface are unaffected.

## Migration

No spec or stored-value migration is required. Behavior note: the deploy form can no longer mark the web chat as public, and redeploying through the form drops any previously set `auth.web.public`, leaving the web chat protected.
