# Reserve marketing route names from account names

## Summary

Astro accounts are addressed as `astropods.ai/{name}` on the apex
distribution. Several marketing routes (`/builders`, `/features`,
`/solutions`, `/customers`, plus case-studies / customer-stories / etc.)
are now served by CloudFront from the marketing origin. If a user
registered an account whose slug collided with one of those paths, the
account would be unreachable: the path is routed to marketing before it
ever hits the app. This change blocks those names at account creation,
and takes the opportunity to sweep in the rest of GitHub's
reserved-route vocabulary so we don't have to do this piecemeal each
time we add a marketing page.

## Design

`internal/account/validate.go` already keeps two maps used by
`CheckAccountNameRestricted`:

- `reservedNames` — primary slugs that conflict with frontend, backend,
  or marketing routes.
- `reservedVariants` (in `variants.go`) — singular/plural completions
  of `reservedNames` so e.g. blocking `customers` also blocks
  `customer`.

We extend both. Three buckets:

1. **Marketing routes we are actively adding.** `builders`, `features`,
   `solutions`, `case-studies`, `customer-stories`, `investors`,
   `nonprofits`, `shareholders`, `showcases`, `site-policy`,
   `social-impact`. (`customers`, `pricing`, `legal`, `enterprise`,
   `about`, `downloads` were already reserved.) These mirror the
   CloudFront apex routing list in
   `modules/astro-infra/terraform/environments/{prod,preview}/cloudfront_apex.tf`.

2. **Safety / abuse slugs.** `abuse`, `malware`, `spam`, `suspended`,
   `invalid-email-address`. Not in use today, but common
   impersonation/phishing handles and cheap to block now.

3. **Generic web/SaaS routes from GitHub's reserved-names list**
   (`github-reserved-names`, the standard reference). Across the
   existing sections we add the GitHub vocabulary that fits a SaaS
   product: `anonymous`, `identity`, `sessions`, `username` (auth);
   `individual`, `personal` (user); `apps`, `cloud`, `design`,
   `editor(s)`, `images`, `library`, `listings`, `lists`, `mobile`,
   `payments`, `releases`, `services`, `stories`, `subscriptions`,
   `topic`/`topics` (product features); `info`, `journal(s)`, `learn`,
   `news`, `newsletter`, `newsroom`, `pages`, `posts`, `readme`,
   `resources`, `site`, `sitemap`, `talks`, `timeline`, `translations`,
   `trending`, `updates`, `wiki(s)` (content); `garage`, `lab(s)`,
   `ssh` (developer); `business(es)`, `launch`, `offer`, `popular`,
   `professional`, `sponsors`, `staff`, `store`, `tour` (marketing);
   `suggest(ion[s])`, `train(ing)`, `watching` (verbs);
   `downtime`, `linux`, `mac`, `windows` (infra).

   GitHub-product-specific names (`copilot`, `codespaces`, `gist`,
   `mention(s/ed/ing)`, `starred`, `stars`, `hovercards`,
   `save-net-neutrality`, `why-github`, etc.), git-vocabulary names
   (`blob`, `tree`, `branches`, `commits`, `pulls`, `raw`, `issues`,
   `milestones`, etc.), and HTTP-numeric reserved names are
   deliberately omitted. Numerics are already blocked by
   `ValidateAccountName`'s "must start with a letter" rule.

`reservedVariants` is extended with the corresponding singular/plural
completions (e.g. `solution` for `solutions`, `enterprises` for the
already-reserved `enterprise`, `session` for `sessions`, `page` for
`pages`, etc.).

The validation logic itself does not change — both maps are still
checked by `CheckAccountNameRestricted` and return the same
"name is reserved" error.

## Migration

No migration. Existing accounts are unaffected — `ValidateAccountName`
is called on creation, not on lookup. New signups attempting to claim
any of the newly reserved slugs will be rejected with the existing
"name is reserved" error.
