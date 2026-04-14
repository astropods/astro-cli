# Fix missing viewport meta tag and add page titles

## Summary

iOS Safari rendered pages at a zoomed-out desktop scale because child routes in React Router v7 replace parent `meta` entirely. The root viewport and charset tags were defined in root's `meta` export, so any page with its own `meta` function silently dropped them. Additionally, most pages had no `meta` export at all, leaving the browser tab stuck on the generic "Astro" title.

## Design

**Viewport fix** -- Moved `charset` and `viewport` out of root's `meta` function and into hardcoded `<meta>` tags in the root `Layout` `<head>`. These are now always present regardless of child route meta behavior.

**Page titles** -- Added `meta` exports to every leaf route that was missing one. Titles follow the pattern `Page Name | Astro`, with settings pages using `Section - Settings | Astro` and org settings using `Section - Organization Settings | Astro`. Dynamic pages (AccountProfile, AccountBlueprints) interpolate route params.

| Page | Title |
|------|-------|
| NotFound | Page Not Found \| Astro |
| RequestBlueprint | Request Agent \| Astro |
| NewBlueprint | New Agent \| Astro |
| AgentDashboard | Dashboard \| Astro |
| Onboarding | Get Started \| Astro |
| Admin | Admin \| Astro |
| AccountProfile | {account} \| Astro |
| OrganizationNew | New Organization \| Astro |
| AccountSettings | Account - Settings \| Astro |
| UsageSettings | Usage - Settings \| Astro |
| SecretsSettings | Secrets - Settings \| Astro |
| OrganizationsSettings | Organizations - Settings \| Astro |
| ExperimentsSettings | Experiments - Settings \| Astro |
| OrgGeneralSettings | General - Organization Settings \| Astro |
| OrgMembersSettings | Members - Organization Settings \| Astro |
| OrgSecretsSettings | Secrets - Organization Settings \| Astro |
| ConfigureDeployment | Configure Deployment \| Astro |
| ConfigureDangerZone | Danger Zone \| Astro |
| Personal (blueprints) | My Blueprints \| Astro |
| AccountBlueprints | {account} Blueprints \| Astro |

## Migration

No migration required.
