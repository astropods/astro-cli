# Deployment author visibility

Ticket: [#1121](https://github.com/astropods/astro/issues/1121)

## Summary

Deployment history now identifies the account member who initiated each deployment, making ownership visible across the active deployment and earlier revisions.

## Design

Deployment responses distinguish deploy actors from people who performed later operational actions. Historical revisions are correlated with their matching `deployment.deploy` audit event, and the client resolves every actor through the existing account-members query. Only resolved team members appear, with their profile-linked avatar and preferred display name; admin actors and removed members remain hidden.

## Migration

No migration is required.
