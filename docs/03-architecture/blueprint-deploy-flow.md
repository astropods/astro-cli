# Blueprint deploy flow: browse, detail, configure, deploy

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

This doc covers the astro-client pages and components a user moves through
to find a blueprint, look at it, and deploy it: browse → detail → deploy,
plus creating a new blueprint and reconfiguring an existing deployment. For
the registry these pages read from and write to (agent/version/visibility
data model, CLI push, GitHub build registration), see
[`blueprint-lifecycle.md`](blueprint-lifecycle.md). For what happens on the
server after the deploy call is made, see
[`deployment-state-machine.md`](deployment-state-machine.md). The
Variables/secrets vault UI (`VaultPicker`, `VariableField*`) plugs into the
deploy form described here but isn't itself documented yet — not yet
documented, no link to give.

---

## Route map

From `apps/astro-client/src/routes.ts`:

| Route | Page | Role |
|---|---|---|
| `blueprints` | `pages/blueprints/Blueprints.tsx` | Browse |
| `:account/:agentSlug` (route id `agent-detail`) | `pages/BlueprintDetail.tsx` | Detail |
| `getting-started`, `new/custom` | `pages/NewBlueprint.tsx` | Create a blueprint |
| `deploy/:account/:agentSlug` | `pages/DeployBlueprint.tsx` | Deploy (new deployment) |
| `:account/agents/:deploymentId/configure` | `pages/agent-detail/AgentConfigure.tsx` | Configure/redeploy an *existing* deployment |

`apps/astro-client/src/pages/configure/**` (`ConfigureRedirect.tsx`,
`ConfigureDangerZone.tsx`, `types.ts`) is **not wired into `routes.ts` at
all**. `ConfigureRedirect.tsx` says so directly:

```tsx
// NOTE: This file is no longer routed (replaced by agent-detail routes).
// Kept temporarily as reference while the new page is built.
```

It redirects to `` `/${account}/agents/${deploymentId}/configure/deployment` ``,
an account/deployment-specific path built from route params, which doesn't
match any current route. `ConfigureDangerZone.tsx` (delete-deployment UI) and
`types.ts` are only referenced by each other, not by anything live. Treat
`pages/configure/**` as an orphaned migration artifact, not as the current
implementation — the real configure page is `AgentConfigure.tsx`.

---

## Page and component hierarchy

### Browse (`pages/blueprints/Blueprints.tsx`)

`PageContainer`/`PageHeader` → filter toolbar (`DebouncedFilterInput` +
`AccountFilter`) → `ListResultsTransition` → `BlueprintListView`
(`components/browse/BlueprintListView.tsx`, grid or list layout of
`BlueprintCard`) → `ListPagination`. Empty states are `FilteredEmptyState`
(a search/filter matched nothing) or `BlueprintsEmptyState`
(`components/blueprint/BlueprintsEmptyState.tsx`, the registry itself is
empty for this scope). Search/filter/pagination logic lives in
`pages/blueprints/use-blueprint-search.ts`, backed by cursor pagination
(`useCursorPagination`) and a persisted account-scope filter
(`usePersistentAccountFilterParam`).

### Detail (`pages/BlueprintDetail.tsx`)

`BlueprintDetailBreadcrumb` → `BlueprintDetailContent` (README, setup
instructions, `BlueprintDetailHeader`) alongside `BlueprintDetailSidebar`
(desktop) / `SidebarCard` (mobile equivalent). The sidebar renders
`SidebarAuthor`, `SidebarRepository`/`GitHubConnectionPanel`,
`SidebarStats`, `CapabilitiesList`, `RequiredAppsList`,
`SidebarDeployedAgents`, and a recommended-agents list — all under
`components/blueprint-detail/`.

The deploy entry point is a plain link in the sidebar:

```tsx
<Link to={`/deploy/${agent.account}/${agent.name}`}>Deploy this agent</Link>
```

Nothing on the detail page pre-fetches deploy-form state; `DeployBlueprint`
loads independently once navigated to.

### Create (`pages/NewBlueprint.tsx`)

A self-contained four-step wizard: `STEPS = ["setup", "source", "publishing",
"review"]`, rendered as a sliding carousel. `setup` collects name/visibility
(validated by `validateSetup`, including a live uniqueness probe via
`useBlueprint`). `source` decides between a fresh empty blueprint or
importing from a GitHub repo (`RepoPicker`, `LinkConfirmDialog` — both in
`components/new-blueprint/`), tracked in a `useReducer` (`sourceReducer`)
holding `{ sourcePath, githubConnected, scanResult }`. Because a GitHub
OAuth connect navigates the browser away and back, in-progress wizard state
round-trips through `sessionStorage` under the key
`astro:new-blueprint-wizard`. `review` polls `useBlueprint` every 5 seconds
waiting for the user's first `ast push` to land.

Both `setup` and `source` validation run through a shared `gate(fieldErrors)`
helper that drives the Continue/Create button — the button submits and
surfaces errors rather than staying disabled.

### Deploy (`pages/DeployBlueprint.tsx`) and Configure (`pages/agent-detail/AgentConfigure.tsx`)

**These are the same form, not two implementations.** Both pages call the
same `useDeployForm` hook and render the same `BlueprintVersionPicker` +
`DeployFormFields` components; only the surrounding page chrome and the
options passed to the hook differ:

| | Deploy | Configure |
|---|---|---|
| Purpose | Create a new deployment | Reconfigure/redeploy an existing one |
| `useDeployForm` options | `build` (which version to deploy, from `?build=`) | `deploymentId` plus `build` (build override) and `revision` (rollback), also from URL search params |
| Action row | simple Cancel / Deploy | floating footer, Discard / Save-or-Redeploy |
| Extra sections | — | manual ingestion trigger buttons (`useTriggerIngestion`) |

Both are a single long form (not a wizard); `BlueprintVersionPicker` lets the
user pick which registered version (`build_id`, from
[`blueprint-lifecycle.md`](blueprint-lifecycle.md)) to deploy or roll back
to.

---

## State

| State | Lives in |
|---|---|
| Blueprint data, lists, GitHub connection status | TanStack Query — `useBlueprint`, `useBlueprints`, `useAccountBlueprints`, `useUserBlueprints` (`api/queries/blueprints.ts`), `useGitHubStatus`/`useGitHubAccountStatus` |
| All deploy/configure form fields (name, account, cluster, adapters, variables, grants, provisioning, schedules, knowledge bindings, model choices) | Local `useState` inside `useDeployForm.ts` — a single hook, not TanStack Query and not a global store |
| The server-computed deployment template (schema, defaults, validation) | `useState<TemplateResponse>`, populated by the `usePostDeploymentTemplate` mutation and re-POSTed ("reshaped") whenever a field that affects the template's shape changes (e.g. which adapters are enabled) |
| Deploy submission | `useDeployAgent` mutation; on success invalidates `deploymentKeys.all`, `deploymentKeys.visibleLists`, `blueprintKeys.detail`, `blueprintKeys.byAccount`, and `deploymentKeys.history` unconditionally, plus `deploymentKeys.detail` only when the response includes a `deployment_id` |
| Selected build (Deploy) / build override and rollback revision (Configure) | URL search params (`?build=`, `?revision=`), not component state |
| Browse filters and pagination | URL-persisted (`usePersistentAccountFilterParam`, `useCursorPagination`) |
| New-blueprint wizard step and GitHub-connect sub-state | Local `useState` (`activeStep`) + `useReducer` (`sourceReducer`), OAuth round-trip persisted to `sessionStorage` |
| Staged avatar image (deploy/new-blueprint) | Local `useState`/`useRef`, uploaded only after the create/deploy call succeeds |

Relevant query hooks (`src/api/queries/blueprints.ts`, `deployments.ts`,
`knowledge.ts`):

- `useUserBlueprints`, `useBlueprints`, `useAccountBlueprints`, `useBlueprint`
  — `useBlueprint` has no poll interval of its own; it only forwards
  `opts.refetchInterval` when a caller passes one. Two callers do:
  `NewBlueprint.tsx`'s review step passes a fixed 5-second interval (see
  [Create](#create-pagesnewblueprinttsx) above), and `BlueprintDetail.tsx`
  passes a function that polls every 10 seconds while the blueprint has zero
  versions and stops once a version appears
- `usePostDeploymentTemplate`, `useDeployAgent`, `useValidateDeployment`
- `useCreateBlueprint`, `useArchiveBlueprint`
- `useUploadBlueprintAvatar`
- `useKnowledgeStores` (feeds the deploy form's Knowledge section)

Query keys come from the `blueprintKeys`/`deploymentKeys`/`githubKeys`
factories in `api/queries/keys.ts`, per the project's TanStack Query
conventions.

---

## The deploy form

`DeployFormFields.tsx` renders these sections, each conditional on what the
current template response says the blueprint needs:

1. **General** — avatar upload, agent name, cluster (`ClusterPicker`),
   target account (`AccountPicker`, hidden if there's only one account to
   choose from).
2. **Model** — one select per model the blueprint's spec declares, if any.
3. **Messaging interface** — adapter checkboxes (`InterfacesPicker`), each
   adapter's own credential fields, and access grants
   (`GrantsEditor`/`AddGrantMenu`/`GrantRow`/`MemberPicker`) for web/Slack
   auth.
4. **Custom interface** — public toggle plus its own access control grants.
5. **Knowledge** — `KnowledgeBindingPicker`, populated from
   `useKnowledgeStores`.
6. **Configuration** (required variables) and **optional credentials** —
   both rendered by `VariableFields` over the template's variable list, with
   a paste-a-`.env`-file bulk import action.
7. **Resources** — CPU, memory, volume mount path and size (locked once a
   volume is already provisioned), response timeout.
8. **Ingestion** — a schedule picker per scheduled ingestion source, plus
   (Configure only) manual trigger buttons.

**This is where the vault UI plugs in.** Each `VariableField`
(`components/deploy/VariableField.tsx`) that represents a secret-shaped
field renders `VaultPicker`/`VaultRefChip` (imported from
`components/deploy/VaultPicker.tsx`) so the user can bind an existing
account secret instead of typing a raw value. That vault system (how
secrets are stored, referenced, and resolved) isn't documented separately
yet — this doc only notes the integration point.

### Submit: a two-phase call

`useDeployForm.ts`'s submit path never sends form state directly to the
deploy endpoint. It goes through the same signed-template mechanism
described in [`blueprint-lifecycle.md`](blueprint-lifecycle.md#from-a-registered-version-to-a-deploy):

1. Build a `TemplateRequest` from form state and POST it via
   `usePostDeploymentTemplate` with `finalize: true`:
   ```ts
   const req: TemplateRequest = {
     interfaces: buildInterfaces(),       // { adapters, auth: { web, slack, custom } }
     variables: variableInputs,           // Record<key, { value?, ref? }>
     schedules: ingestionSchedules,
     bindings: { knowledge: nonEmptyBindings(knowledgeBindings) },
   };
   // + models, deployment_id, build, revision, provisioning, cluster_id as applicable
   req.finalize = true;
   ```
2. The server returns `{ template, validation: { valid, errors }, signature }`.
   If `validation.valid` is false, the form surfaces field errors and stops
   — there's no second call.
3. On success, the form patches only the target fields (account, display
   name) into the returned template and submits:
   ```ts
   const spec: DeploymentSpec = {
     ...resp.template,
     target: { ...resp.template.target, account: targetAccount, display_name: deployName.trim() },
   };
   await deployMutation.mutateAsync({ spec, signature: resp.signature });
   ```

The `signature` is the HMAC produced by `specsign` on the server; echoing it
back on the deploy call is what lets the server trust the spec wasn't
hand-edited between steps 1 and 3, without re-validating every field. See
`blueprint-lifecycle.md` for what `specsign` actually covers (it excludes
the target fields the form patches in step 3, on purpose).

### Validation

No schema library (no zod) — validation is hand-written:

- `NewBlueprint.tsx`: `validateSetup` (name required, minimum length, must
  start with a letter, live uniqueness check) and `validateSource`
  (repo/GitHub-connection requirements), both gating the wizard's Continue
  button through `gate(fieldErrors)`.
- `useDeployForm.ts`: `trySubmit()` checks account/name presence and length,
  required variables filled (`isVariableFilled`), adapter credentials
  filled, at least one ingestion schedule if ingestion is enabled, vault
  refs resolvable, and required knowledge bindings satisfied — plus a
  parallel `errors` computation that only surfaces inline messages after a
  first submit attempt.

No feature-flag SDK (LaunchDarkly or otherwise) gates any part of this flow.

---

## Real complexity worth knowing about

- **Deploy and Configure share one form implementation.** Any change to
  `useDeployForm`, `DeployFormFields`, or `BlueprintVersionPicker` affects
  both creating a new deployment and reconfiguring/rolling back an existing
  one — there's no independent "configure" code path to keep in sync.
- **The template "reshape" mechanic**: changing certain fields (adapters,
  knowledge bindings) re-POSTs the template mid-edit to get updated
  variable optionality/defaults, without discarding values the user already
  typed into fields the reshape doesn't affect.
- **Configure supports both build override and rollback** via the same
  `TemplateRequest.build`/`.revision` fields, sourced from URL search
  params (`?build=` is transient and cleared from the URL once consumed;
  `?revision=` persists for rollback).
- **`pages/configure/**` is dead/orphaned**, not a second implementation to
  reconcile with — see [Route map](#route-map). Its
  `ConfigureDangerZone.tsx` (delete-deployment UI) currently has no live
  route pointing at it; deployment deletion, if it exists in the shipped UI,
  lives somewhere this doc didn't trace — worth a follow-up check if you're
  touching deployment deletion.
- **`AdvancedProvisioningFields`'s CPU/memory pricing is a placeholder** —
  the component has hardcoded USD unit prices with a comment noting they
  should be replaced once real billing data is available from the API.
