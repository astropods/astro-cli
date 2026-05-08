import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import AccountProfile from './AccountProfile';
import type { AccountPublic, BlueprintsListResponse } from '@/lib/api';

afterEach(cleanup);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockAccount: AccountPublic = {
  id: 'acct-1',
  name: 'testuser',
  type: 'personal',
  display_name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

// Server returns blueprints in this order: gamma → alpha → beta.
// Sort tests prove the client re-orders them correctly.
//   newest:       alpha (Mar) > gamma (Feb) > beta (Jan)
//   name A–Z:     alpha < beta < gamma
//   most deployed: alpha (10) > beta (5) > gamma (1)
const blueprints: BlueprintsListResponse['agents'] = [
  {
    name: 'gamma-bot',
    account: 'testuser',
    registry: 'reg.example.com',
    visibility: 'public',
    versions: [{ build_id: 'b3', published_at: '2025-02-01T00:00:00Z', spec: {} }],
    metrics: { deploy_count: 1, lifetime_messages: 0 },
  },
  {
    name: 'alpha-bot',
    account: 'testuser',
    registry: 'reg.example.com',
    visibility: 'public',
    versions: [{ build_id: 'b1', published_at: '2025-03-01T00:00:00Z', spec: {} }],
    metrics: { deploy_count: 10, lifetime_messages: 0 },
  },
  {
    name: 'beta-bot',
    account: 'testuser',
    registry: 'reg.example.com',
    visibility: 'public',
    versions: [{ build_id: 'b2', published_at: '2025-01-01T00:00:00Z', spec: {} }],
    metrics: { deploy_count: 5, lifetime_messages: 0 },
  },
];

// Agents: server returns zeta (newer updated_at) before alpha (older).
// Name A–Z must put alpha before zeta regardless of server order.
const deployments = [
  {
    id: 'd1',
    name: 'zeta-agent',
    display_name: 'Zeta Agent',
    status: 'Running',
    replicas: 1,
    ready: 1,
    build_id: 'b1',
    namespace: 'ns1',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-03-01T00:00:00Z',
    components: [],
  },
  {
    id: 'd2',
    name: 'alpha-agent',
    display_name: 'Alpha Agent',
    status: 'Running',
    replicas: 1,
    ready: 1,
    build_id: 'b2',
    namespace: 'ns2',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    components: [],
  },
];

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderProfile() {
  return renderRoute(
    [{
      path: '/:account',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      Component: AccountProfile as any,
      loader: () => ({ account: mockAccount, blueprints: null, orgs: null, deployments: null }),
    }],
    { initialEntries: ['/testuser'], auth: mockAuthContext },
  );
}

const BLUEPRINT_NAMES = ['alpha-bot', 'beta-bot', 'gamma-bot'];

/** Returns blueprint card heading names in their current DOM order. */
function getBlueprintOrder(): string[] {
  return screen
    .getAllByRole('heading')
    .map((el) => el.textContent?.trim() ?? '')
    .filter((name) => BLUEPRINT_NAMES.includes(name));
}

// ── Blueprint sorts ───────────────────────────────────────────────────────────

describe('IndividualProfile blueprint sort: Newest', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: blueprints, count: 3 }),
      ),
    );
  });

  it('default sort orders by most recent published_at descending', async () => {
    renderProfile();

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'alpha-bot' })).toBeInTheDocument(),
    );

    // alpha (Mar) → gamma (Feb) → beta (Jan) — different from server order
    expect(getBlueprintOrder()).toEqual(['alpha-bot', 'gamma-bot', 'beta-bot']);
  });

  it('picks the latest version when a blueprint has multiple versions', async () => {
    const multiVersion = [
      {
        ...blueprints[0], // gamma-bot, base published_at: Feb
        name: 'multi-bot',
        versions: [
          { build_id: 'v3', published_at: '2025-04-01T00:00:00Z', spec: {} }, // newest
          { build_id: 'v2', published_at: '2025-02-01T00:00:00Z', spec: {} },
          { build_id: 'v1', published_at: '2025-01-01T00:00:00Z', spec: {} },
        ],
      },
      blueprints[1], // alpha-bot: Mar
    ];

    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: multiVersion, count: 2 }),
      ),
    );

    renderProfile();
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'multi-bot' })).toBeInTheDocument(),
    );

    // multi-bot (Apr) should come before alpha-bot (Mar)
    const order = screen
      .getAllByRole('heading')
      .map((el) => el.textContent?.trim() ?? '')
      .filter((n) => ['multi-bot', 'alpha-bot'].includes(n));
    expect(order).toEqual(['multi-bot', 'alpha-bot']);
  });
});

describe('IndividualProfile blueprint sort: Name A–Z', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: blueprints, count: 3 }),
      ),
    );
  });

  it('sorts alphabetically by name', async () => {
    const user = userEvent.setup();
    renderProfile();

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'alpha-bot' })).toBeInTheDocument(),
    );

    await user.click(screen.getByRole('button', { name: /newest/i }));
    await user.click(screen.getByRole('menuitem', { name: /name a/i }));

    await waitFor(() =>
      expect(getBlueprintOrder()).toEqual(['alpha-bot', 'beta-bot', 'gamma-bot']),
    );
  });
});

describe('IndividualProfile blueprint sort: Most deployed', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: blueprints, count: 3 }),
      ),
    );
  });

  it('sorts by deploy_count descending', async () => {
    const user = userEvent.setup();
    renderProfile();

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'alpha-bot' })).toBeInTheDocument(),
    );

    await user.click(screen.getByRole('button', { name: /newest/i }));
    await user.click(screen.getByRole('menuitem', { name: /most deployed/i }));

    // alpha (10) → beta (5) → gamma (1)
    await waitFor(() =>
      expect(getBlueprintOrder()).toEqual(['alpha-bot', 'beta-bot', 'gamma-bot']),
    );
  });

  it('treats missing deploy_count as zero', async () => {
    const withMissingCount = [
      { ...blueprints[1], name: 'has-count', metrics: { deploy_count: 3, lifetime_messages: 0 } },
      { ...blueprints[0], name: 'no-count', metrics: undefined },
    ];
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: withMissingCount, count: 2 }),
      ),
    );

    const user = userEvent.setup();
    renderProfile();

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'has-count' })).toBeInTheDocument(),
    );

    await user.click(screen.getByRole('button', { name: /newest/i }));
    await user.click(screen.getByRole('menuitem', { name: /most deployed/i }));

    await waitFor(() => {
      const order = screen
        .getAllByRole('heading')
        .map((el) => el.textContent?.trim() ?? '')
        .filter((n) => ['has-count', 'no-count'].includes(n));
      expect(order).toEqual(['has-count', 'no-count']);
    });
  });
});

// ── Agent sort ────────────────────────────────────────────────────────────────

describe('IndividualProfile agent sort: Name A–Z', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ deployments, count: 2 }),
      ),
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: [], count: 0 }),
      ),
    );
  });

  it('sorts agents alphabetically by name', async () => {
    const user = userEvent.setup();
    renderProfile();

    // Click the Agents tab
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /^agents/i })).toBeInTheDocument(),
    );
    await user.click(screen.getByRole('button', { name: /^agents/i }));

    // Wait for both agent cards to appear (server returns zeta before alpha)
    await waitFor(() => {
      expect(screen.getByText('Zeta Agent')).toBeInTheDocument();
      expect(screen.getByText('Alpha Agent')).toBeInTheDocument();
    });

    // Sort by Name A–Z
    await user.click(screen.getByRole('button', { name: /last modified/i }));
    await user.click(screen.getByRole('menuitem', { name: /name a/i }));

    await waitFor(() => {
      const body = document.body.textContent ?? '';
      expect(body.indexOf('Alpha Agent')).toBeLessThan(body.indexOf('Zeta Agent'));
    });
  });

  it('default (Last modified) keeps server order', async () => {
    const user = userEvent.setup();
    renderProfile();

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /^agents/i })).toBeInTheDocument(),
    );
    await user.click(screen.getByRole('button', { name: /^agents/i }));

    await waitFor(() => {
      expect(screen.getByText('Zeta Agent')).toBeInTheDocument();
      expect(screen.getByText('Alpha Agent')).toBeInTheDocument();
    });

    // No sort applied — server order: zeta before alpha
    const body = document.body.textContent ?? '';
    expect(body.indexOf('Zeta Agent')).toBeLessThan(body.indexOf('Alpha Agent'));
  });
});
