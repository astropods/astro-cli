# Scope switchers remain available in empty states

## Summary

The Agents, Blueprints, and Knowledge Stores pages now keep their account scope switchers visible when the default personal account has no resources. Users can move directly from onboarding empty states to any account they belong to.

## Design

Personal-account onboarding states retain their focused empty-state presentation, with the account switcher rendered above them. The switcher continues to use the authenticated membership list, so every account appears regardless of whether that account has agents, blueprints, or knowledge stores. Search and sort controls remain hidden until resources exist or a filter is explicit.

## Migration

No migration is required.
