import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { server } from '@/test/msw/server';
import type { AuthContextType } from '@/lib/auth-context';
import type { AuditLogListResponse, AuditLogFilterOptions } from '@/lib/api';
import OrgAuditLogSettings from '@/pages/settings/OrgAuditLogSettings';
import AuditLogSettings from '@/pages/settings/AuditLogSettings';

afterEach(cleanup);

// ── Fixtures ─────────────────────────────────────────────────────────

const ORG_SLUG = 'test-org';

const orgAccount = {
  id: 'org-1',
  name: ORG_SLUG,
  type: 'organization' as const,
  organization_id: 'wos-org-1',
};

const makeAuth = (role: string): AuthContextType => ({
  ...mockAuthContext,
  role,
  organizationId: 'wos-org-1',
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal' },
    orgAccount,
  ],
});

const members = [
  {
    account_id: 'org-1',
    user_id: 'user-1',
    role: 'admin',
    status: 'active',
    username: 'testuser',
    display_name: 'Test User',
    created_at: '2025-01-01T00:00:00Z',
  },
  {
    account_id: 'org-1',
    user_id: 'user-2',
    role: 'member',
    status: 'active',
    username: 'otheruser',
    display_name: 'Other User',
    created_at: '2025-01-02T00:00:00Z',
  },
];

const auditEntries: AuditLogListResponse = {
  entries: [
    {
      id: 1,
      actor: { id: 'user-1', type: 'user' },
      action: 'deployment.deploy',
      resource: { type: 'deployment', id: 'dep-1', name: 'my-agent' },
      description: 'Deployed agent my-agent',
      created_at: '2025-04-20T10:00:00Z',
    },
    {
      id: 2,
      actor: { id: 'user-2', type: 'user' },
      action: 'agent.register',
      resource: { type: 'agent', id: 'agent-1', name: 'my-agent' },
      description: 'Registered agent my-agent',
      created_at: '2025-04-19T10:00:00Z',
    },
    {
      id: 3,
      actor: { id: 'admin:grpc', type: 'admin' },
      action: 'deployment.restart',
      resource: { type: 'deployment', id: 'dep-1', name: 'my-agent' },
      description: 'Restarted deployment my-agent',
      created_at: '2025-04-18T10:00:00Z',
    },
  ],
  has_more: false,
};

const auditFilters: AuditLogFilterOptions = {
  resource_types: ['agent', 'deployment'],
  actions: ['agent.register', 'deployment.deploy', 'deployment.restart'],
};

// ── Helpers ──────────────────────────────────────────────────────────

function useMocks(
  entries = auditEntries,
  filters = auditFilters,
) {
  server.use(
    http.get('*/api/v1/accounts/:account/audit-log/filters', () =>
      HttpResponse.json(filters),
    ),
    http.get('*/api/v1/accounts/:account/audit-log', () =>
      HttpResponse.json(entries),
    ),
    http.get('*/api/v1/accounts/:account/members', () =>
      HttpResponse.json({ members }),
    ),
  );
}

function renderOrgPage(role = 'admin') {
  return renderRoute(
    [{ path: '/settings/org/:orgSlug/audit-log', Component: OrgAuditLogSettings }],
    {
      initialEntries: [`/settings/org/${ORG_SLUG}/audit-log`],
      auth: makeAuth(role),
    },
  );
}

function renderPersonalPage() {
  return renderRoute(
    [{ path: '/settings/audit-log', Component: AuditLogSettings }],
    {
      initialEntries: ['/settings/audit-log'],
      auth: mockAuthContext,
    },
  );
}

// ── Tests ────────────────────────────────────────────────────────────

describe('AuditLogView', () => {
  describe('rendering entries', () => {
    beforeEach(() => useMocks());

    it('renders audit log entries with descriptions and actions', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('Deployed agent my-agent')).toBeInTheDocument();
      });
      expect(screen.getByText('Registered agent my-agent')).toBeInTheDocument();
      expect(screen.getByText('Restarted deployment my-agent')).toBeInTheDocument();
      expect(screen.getByText('deployment.deploy')).toBeInTheDocument();
      expect(screen.getByText('agent.register')).toBeInTheDocument();
    });

    it('resolves user actors to display names', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('Test User')).toBeInTheDocument();
      });
      expect(screen.getByText('Other User')).toBeInTheDocument();
    });

    it('shows admin actor ID directly', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('admin:grpc')).toBeInTheDocument();
      });
    });

    it('shows resource names and types', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getAllByText('my-agent').length).toBeGreaterThanOrEqual(1);
      });
      expect(screen.getAllByText('deployment').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('agent').length).toBeGreaterThanOrEqual(1);
    });
  });

  describe('empty state', () => {
    it('shows empty state when no entries', async () => {
      useMocks({ entries: [], has_more: false }, { resource_types: [], actions: [] });
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('No events yet')).toBeInTheDocument();
      });
    });
  });

  describe('filter toolbar', () => {
    beforeEach(() => useMocks());

    it('renders search input', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByPlaceholderText('Search events...')).toBeInTheDocument();
      });
    });

    it('renders resource type filter with server options', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('All resources')).toBeInTheDocument();
      });
    });

    it('renders action filter with server options', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('All actions')).toBeInTheDocument();
      });
    });

    it('renders actor filter with member names', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('All actors')).toBeInTheDocument();
      });
    });

    it('shows results summary', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText(/Showing 3 entries/)).toBeInTheDocument();
      });
    });
  });

  describe('pagination', () => {
    it('shows Load more button when has_more is true', async () => {
      useMocks({
        ...auditEntries,
        has_more: true,
        next_before: '2025-04-18T10:00:00Z,3',
      });
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /load more/i })).toBeInTheDocument();
      });
    });

    it('hides Load more button when has_more is false', async () => {
      useMocks();
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('Deployed agent my-agent')).toBeInTheDocument();
      });
      expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument();
    });

    it('shows + in results summary when more pages exist', async () => {
      useMocks({
        ...auditEntries,
        has_more: true,
        next_before: '2025-04-18T10:00:00Z,3',
      });
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText(/Showing 3 entries\+/)).toBeInTheDocument();
      });
    });
  });

  describe('personal account page', () => {
    beforeEach(() => useMocks());

    it('renders with personal account subtitle', async () => {
      renderPersonalPage();
      await waitFor(() => {
        expect(screen.getByText('A record of actions taken on your account')).toBeInTheDocument();
      });
    });

    it('shows audit log entries', async () => {
      renderPersonalPage();
      await waitFor(() => {
        expect(screen.getByText('Deployed agent my-agent')).toBeInTheDocument();
      });
    });
  });

  describe('table headers', () => {
    beforeEach(() => useMocks());

    it('renders all four column headers', async () => {
      renderOrgPage();
      await waitFor(() => {
        expect(screen.getByText('Event')).toBeInTheDocument();
      });
      expect(screen.getByText('Resource')).toBeInTheDocument();
      expect(screen.getByText('Actor')).toBeInTheDocument();
      expect(screen.getByText('Time')).toBeInTheDocument();
    });
  });
});
