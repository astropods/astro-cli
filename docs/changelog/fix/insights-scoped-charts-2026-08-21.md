# Scope the Claude Code page to what the reader may see

## Summary

Two defects let the source detail page report the wrong thing to the wrong people.

A reader without permission to see their colleagues still got account-wide charts — only the named breakdown below them was restricted. The charts are built from everyone's classified prompts, so they reported the account's work/personal split to someone not allowed to see who was behind it.

Separately, the gate admitted almost nobody it was meant to. It asked for `org:admin`, which WorkOS grants to the `owner` role only, so every org **admin** was restricted to their own row. And because the check reads permissions from the session, a caller viewing an organization their session was not scoped to failed it outright — owners included.

## Design

**The restriction scopes the page, not a section.** It is applied in SQL when reading the daily aggregates, so a reader who may not see their colleagues never has those rows in process and every surface built from them — charts, series, breakdown — describes only what that reader may see. The previous split kept the restriction in Go because the charts needed every actor's rows; scoping the charts removes that requirement.

**Elevation is `org:manage`.** Both `admin` and `owner` carry it, and it is what the rest of the account-administration checks already use. `org:admin` exists on the owner role alone:

```
member   []
admin    [... org:manage ...]
owner    [... org:manage, org:admin ...]
```

**A session scoped to another organization no longer reads as unprivileged.** The account switcher moves the `?account=` param without performing a WorkOS org switch, so viewing one of your other organizations carries a session pointed elsewhere. Where that happens the caller's role in the account's organization is resolved from WorkOS instead of read from the session. That is the same authority as the session claim, fetched rather than cached, so it widens nothing — it stops the answer depending on which organization the session happens to point at. A session already on that organization is trusted as-is, so a role cannot outrank a permission deliberately withheld from it.

**The unresolved-viewer state now fires.** It keyed off an empty actor key, which the handler always populates, so it was unreachable; a reader with no linked dev-tool address saw an empty table instead of an explanation. It now keys off whether any address on the account resolves to them. With the charts scoped too, that reader would otherwise get a blank page.

**The page is behind an experiment.** `prompt-classification-stats`, off by default. The detail endpoint answers 404 rather than 403 when it is off — to an account without the experiment the page does not exist, and refusing would advertise it — and the Insights row renders without a link, so there is nothing pointing at a route that 404s.

The switch is not organization-only. Fine-grained access is, because it governs roles among members and a single-member account has none; classification runs off a personal account's own telemetry just the same, so restricting it there would lock a solo developer out of the page rather than default them off. It appears in both personal and organization experiment settings, scoped to the account it belongs to.

Adding a second switch generalised the endpoint rather than doubling it: experiments are now addressed by slug against a small registry carrying the stored key, the label used in audit entries, whether the switch is organization-only, and any permission it needs beyond the route's. The fine-grained access URL is unchanged.

That last field exists because the route moved. Experiments now sit on `org:manage` so admins can reach them, but fine-grained access governs deployment privacy — turning it off exposes every synchronized deployment to every member — and that was owner-only before. Rather than widen it as a side effect, the registry keeps `org:admin` on that one switch and the handler enforces it per experiment. Authorization is a property of the switch, not of the URL it shares.

Also removes `handlers/localseed_test.go`, a helper that drives the roll-up and classification against a developer's own database. It was meant to stay out of the previous change and reached `main` when a wholesale `git add` undid the commit that untracked it.

## Migration

No schema change and no new configuration. `prompt-classification-stats` must be enabled per account under **Settings → Experiments** — personal settings for a personal account, organization settings for an organization — before the page is reachable.

## Open items

- The `accountAdmin` route group — account rename and delete — gates on `org:admin` despite a comment reading "owner/admin", so those operations are owner-only. Left alone here rather than widening account-mutation rights as a side effect of an Insights fix.
- Two pages are called Experiments: personal settings holds per-browser localStorage toggles, organization settings holds server-owned per-account switches. This switch is server-owned and appears on both, scoped to the relevant account, but the naming collision predates it and is still confusing.
- The cross-organization role fallback matches on role slug rather than resolving the role's permissions, so it assumes the default mapping. An organization that strips `org:manage` from its admin role would still be treated as elevated there while an in-organization session correctly is not.
- The main Insights page charts stay account-wide for every member. They report agent spend rather than per-developer classification, so the same argument does not automatically carry; its dev-tool breakdown picks up the corrected gate either way.
