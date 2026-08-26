# Private-by-default FGA rollout specification

## Summary

Define the staged migration from organization-membership access to private-by-default account, Blueprint, and deployment authorization.

## Design

The plan treats WorkOS Organization as a vendor-required envelope and Account as Astro's authorization root. It introduces generic resource synchronization, Queen inspection and reset controls, group access, reusable assignment UI, account and creator-role projection, shadow comparison, gated enforcement, and a final dead-code pass across ten reviewable phases.

## Migration

No runtime behavior changes. This PR contains planning documentation only.
