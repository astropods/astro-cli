# Organization access-group APIs

## Summary

Organization administrators can now manage WorkOS groups and their members through Astro APIs. Deployment administrators can assign Viewer, Builder, or Admin to a validated group without the browser calling WorkOS directly.

## Design

Group listing is available to organization members for access-subject discovery; lifecycle and membership mutations retain the existing `org:admin` boundary. Every route also requires the global FGA switch and the organization's Fine-grained access experiment. Disabling either returns not found without calling WorkOS.

Requests use Astro user IDs. Astro resolves the selected user to a same-organization WorkOS membership before adding or removing it, and returns a conflict while that identity is not yet provisioned. Successful WorkOS mutations write access-group audit events; idempotent deletes and membership changes do not create phantom events.

Deployment access accepts `member` and `group` subjects. The PR8.4A domain validates that a group belongs to the deployment's WorkOS organization before changing its role.

## Migration

No database migration or backfill is required. WorkOS Groups and the deployment Viewer, Builder, and Admin roles must exist in environments where fine-grained access is enabled.
