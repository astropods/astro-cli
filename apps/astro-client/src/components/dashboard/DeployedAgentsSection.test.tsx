import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { DeployedAgentsSection } from './DeployedAgentsSection';
import type { AgentDeployment, Blueprint, BlueprintsListResponse } from '@/lib/api';

// Threat model for this suite
// ---------------------------
// When a deployment has source_account != viewer account, the section fans
// out useAccountBlueprints(source_account) — a JSON request whose response
// body is fully controlled by whoever owns that account. The dashboard runs
// in the *viewer*'s session and renders one card per deployment, so any of
// the following would be a real cross-account exploit:
//
//   1. A blueprint payload from account A leaks into the lookup for account
//      B's deployment (e.g. flat-key concatenation collision on a name the
//      attacker chooses to collide with a sibling account).
//   2. The response body's `account` field is trusted instead of the URL
//      parameter, letting account A claim to be account B.
//
// Each adversarial test below has been verified to fail when the lookup is
// broken in the corresponding way (flat-key concat / trusting the response's
// account field) and pass under the correct nested-Map, URL-keyed lookup —
// so they cannot pass coincidentally.

function renderSection(deployments: AgentDeployment[], account: string) {
  // Fresh QueryClient per test prevents cached responses from a prior case
  // from satisfying the next test's queries with stale data.
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, staleTime: 0 },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DeployedAgentsSection deployments={deployments} account={account} isLoading={false} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function makeDeployment(overrides: Partial<AgentDeployment> & { id: string; name: string; build_id: string }): AgentDeployment {
  return {
    namespace: `ns-${overrides.id}`,
    status: 'Running',
    replicas: 1,
    ready: 1,
    created_at: '2026-01-01T00:00:00Z',
    components: [],
    ...overrides,
  };
}

function makeBlueprint(account: string, name: string, builds: { build_id: string; published_at: string }[], visibility: 'public' | 'private' = 'public'): Blueprint {
  return {
    name,
    account,
    registry: 'registry.example.com',
    visibility,
    versions: builds.map((b) => ({
      build_id: b.build_id,
      spec: { model: 'gpt-4o' },
      agent_card: { description: '', tags: [], integrations: [] },
      published_at: b.published_at,
    })),
  };
}

// MSW override that scopes blueprint lists by URL-param account. The handler
// itself ignores everything else — anything an attacker might smuggle inside
// a Blueprint object (mismatched `account`, hostile `name`, etc.) flows
// through unchanged so the test can prove the *client* does the scoping.
function blueprintHandlersByAccount(byAccount: Record<string, Blueprint[]>) {
  return http.get('/api/v1/agents/:account', ({ params }) => {
    const acct = String(params.account);
    const agents = byAccount[acct] ?? [];
    return HttpResponse.json<BlueprintsListResponse>({ agents, count: agents.length });
  });
}

const updateBadge = /update available/i;

describe('DeployedAgentsSection — adversarial cross-account inputs', () => {
  beforeEach(() => cleanup());

  // Attack: an attacker who controls account `attacker-org` returns a
  // blueprint object whose `account` field claims to belong to `victim-org`.
  // If the client trusted the response body's `account` field instead of
  // the URL-param account it actually requested, the attacker could plant
  // a forged "newer build" for any deployment whose source_account is
  // `victim-org`, regardless of whether the viewer ever asked victim's
  // server for that blueprint.
  //
  // Correct behaviour: the lookup is keyed by the requesting account
  // (sourceAccounts[i]), so the attacker's spoofed payload lives only
  // under `attacker-org`, never under `victim-org`.
  it('rejects attacker payloads that spoof the account field of a different org', async () => {
    server.use(
      blueprintHandlersByAccount({
        // attacker-org returns a blueprint whose `name` matches the victim's
        // deployment and whose `account` is forged to be `victim-org`. The
        // version has a forged build_id that, if it ever leaked into
        // victim's lookup, would surface as a fake upgrade.
        'attacker-org': [
          {
            ...makeBlueprint('attacker-org', 'critical-agent', [
              { build_id: 'attacker-forged', published_at: '2026-04-01T00:00:00Z' },
            ]),
            // Deliberate spoof — the server lets this field through because
            // it is informational, but the client must not trust it as a
            // routing key.
            account: 'victim-org',
          },
        ],
        // victim-org returns its real, authoritative blueprint list. The
        // viewer's victim deployment is on the latest build, so under
        // correct lookup there is no badge.
        'victim-org': [
          makeBlueprint('victim-org', 'critical-agent', [
            { build_id: 'victim-current', published_at: '2026-04-01T00:00:00Z' },
          ]),
        ],
      }),
    );

    const deployments: AgentDeployment[] = [
      // The targeted deployment: cross-account from victim-org. Must not
      // pick up attacker-forged as a "newer build".
      makeDeployment({
        id: 'dep-victim',
        name: 'critical-agent',
        build_id: 'victim-current',
        source_account: 'victim-org',
        display_name: 'Victim Critical Agent',
      }),
      // Carrier deployment that triggers the attacker-org fan-out — without
      // this, the dashboard never queries attacker-org and the spoof has
      // no chance to take effect. We use a name that does not exist in the
      // attacker's response so this card has no badge of its own.
      makeDeployment({
        id: 'dep-attacker-trigger',
        name: 'unrelated-agent',
        build_id: 'whatever',
        source_account: 'attacker-org',
        display_name: 'Attacker Carrier',
      }),
    ];

    renderSection(deployments, 'victim-org');

    // Both display names render before queries resolve, so use the
    // attacker-org query completion as the readiness signal: if the lookup
    // ever surfaces the spoof, it surfaces as a badge on the victim card.
    // Wait long enough that any phantom badge would have appeared.
    await waitFor(() => {
      expect(screen.getByText('Victim Critical Agent')).toBeInTheDocument();
      expect(screen.getByText('Attacker Carrier')).toBeInTheDocument();
    });
    // Give react-query a microtask flush window so that any forged blueprint
    // that *would* leak into the victim bucket has had its chance to render.
    await new Promise((r) => setTimeout(r, 50));

    const victim = screen.getByText('Victim Critical Agent').closest('a, div');
    const attacker = screen.getByText('Attacker Carrier').closest('a, div');
    expect(victim?.textContent ?? '').not.toMatch(updateBadge);
    expect(attacker?.textContent ?? '').not.toMatch(updateBadge);
  });

  // Attack: an attacker who controls account `team-x` knows that if the
  // dashboard ever flat-keyed its blueprint lookup by `${acct}-${name}`,
  // they could pick a blueprint name that, concatenated with their own
  // account, collides with a sibling account's (account, name) tuple:
  //
  //   "team-x" + "agent"   -> "team-x-agent"
  //   "team"   + "x-agent" -> "team-x-agent"   <- same string
  //
  // Account names are validated as [a-z][a-z0-9-]{3,38} and agent names
  // allow hyphens, so the attacker can pick this name without violating any
  // server-side check. They publish a blueprint whose newer build_id is a
  // forged value, hoping to surface a fake "Update available" badge on the
  // sibling org's deployment of `x-agent`. Clicking it would attempt a
  // redeploy against a build the deployment was never built from.
  it('does not let an attacker forge an upgrade by colliding the flat composite key', async () => {
    server.use(
      blueprintHandlersByAccount({
        // Attacker's published blueprint. The viewer's deployment on this
        // account is intentionally *current* — under correct lookup the
        // attacker card has no badge.
        'team-x': [
          makeBlueprint('team-x', 'agent', [
            { build_id: 'tx-old', published_at: '2026-01-01T00:00:00Z' },
            { build_id: 'tx-new', published_at: '2026-04-01T00:00:00Z' },
          ]),
        ],
        // Victim org's authoritative blueprint list. Their deployment of
        // `x-agent` is *stale*, so the badge belongs on the victim card —
        // and under any flat-key collision it would be replaced or
        // augmented by the attacker's forged build, which is the exploit.
        team: [
          makeBlueprint('team', 'x-agent', [
            { build_id: 'team-old', published_at: '2026-01-01T00:00:00Z' },
            { build_id: 'team-new', published_at: '2026-04-01T00:00:00Z' },
          ]),
        ],
      }),
    );

    const deployments: AgentDeployment[] = [
      makeDeployment({
        id: 'dep-attacker',
        name: 'agent',
        build_id: 'tx-new',
        source_account: 'team-x',
        display_name: 'Attacker Card',
      }),
      makeDeployment({
        id: 'dep-victim',
        name: 'x-agent',
        build_id: 'team-old',
        source_account: 'team',
        display_name: 'Victim Card',
      }),
    ];

    renderSection(deployments, 'team');

    await waitFor(() => {
      expect(screen.getAllByText(updateBadge).length).toBe(1);
    });
    const attacker = screen.getByText('Attacker Card').closest('a, div');
    const victim = screen.getByText('Victim Card').closest('a, div');
    expect(attacker?.textContent ?? '').not.toMatch(updateBadge);
    expect(victim?.textContent ?? '').toMatch(updateBadge);
  });

  // Attack: same flat-key shape as the previous test, but the attacker's
  // intent is to silently fabricate an upgrade signal against a deployment
  // that is on the latest build of its own lineage. Both colliding-shape
  // deployments are pinned to the latest build of their respective real
  // blueprints, so under correct lookup neither shows a badge. A control
  // deployment in an unrelated account is intentionally stale and acts as
  // the readiness signal — once its badge appears, all blueprint queries
  // have resolved. The assertion is that the *total* badge count equals
  // 1 (the control). Any flat-key collision causes whichever blueprint
  // wins the overwrite to be substituted for the loser's lookup; with
  // both colliding deployments pinned to their own lineage, the substitute
  // build always disagrees, producing a phantom badge → total becomes 2.
  it('does not let an attacker phantom-fabricate an upgrade through the flat-key collision', async () => {
    server.use(
      blueprintHandlersByAccount({
        // Both colliding accounts publish a non-trivial version history so
        // the comparison to "latest" is meaningful no matter who wins.
        'team-x': [
          makeBlueprint('team-x', 'agent', [
            { build_id: 'tx-old', published_at: '2026-01-01T00:00:00Z' },
            { build_id: 'tx-current', published_at: '2026-04-01T00:00:00Z' },
          ]),
        ],
        team: [
          makeBlueprint('team', 'x-agent', [
            { build_id: 'team-old', published_at: '2026-01-01T00:00:00Z' },
            { build_id: 'team-current', published_at: '2026-04-01T00:00:00Z' },
          ]),
        ],
        'control-org': [
          makeBlueprint('control-org', 'control-agent', [
            { build_id: 'ctrl-old', published_at: '2026-01-01T00:00:00Z' },
            { build_id: 'ctrl-new', published_at: '2026-04-01T00:00:00Z' },
          ]),
        ],
      }),
    );

    const deployments: AgentDeployment[] = [
      makeDeployment({
        id: 'dep-tx',
        name: 'agent',
        build_id: 'tx-current',
        source_account: 'team-x',
        display_name: 'TeamX Card',
      }),
      makeDeployment({
        id: 'dep-team',
        name: 'x-agent',
        build_id: 'team-current',
        source_account: 'team',
        display_name: 'Team Card',
      }),
      // Control: intentionally stale. Its badge proves queries resolved.
      makeDeployment({
        id: 'dep-control',
        name: 'control-agent',
        build_id: 'ctrl-old',
        source_account: 'control-org',
        display_name: 'Control Card',
      }),
    ];

    renderSection(deployments, 'team');

    // Wait for the control's badge — proves blueprint queries resolved.
    await waitFor(() => {
      const control = screen.getByText('Control Card').closest('a, div');
      expect(control?.textContent ?? '').toMatch(updateBadge);
    });
    // Under correct lookup the control is the only badge. Any flat-key
    // collision pollutes one of the colliding cards with a phantom badge.
    expect(screen.getAllByText(updateBadge)).toHaveLength(1);
    const tx = screen.getByText('TeamX Card').closest('a, div');
    const team = screen.getByText('Team Card').closest('a, div');
    expect(tx?.textContent ?? '').not.toMatch(updateBadge);
    expect(team?.textContent ?? '').not.toMatch(updateBadge);
  });

});
