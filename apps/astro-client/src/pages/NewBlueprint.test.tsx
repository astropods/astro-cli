import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import type { Blueprint } from '@/lib/api';
import type { AuthContextType } from '@/lib/auth-context';
import NewBlueprint from './NewBlueprint';

afterEach(cleanup);

const ACCOUNT = 'testuser';
const NAME = 'my-agent';

function renderNewBlueprint(options?: { auth?: AuthContextType }) {
  return renderRoute(
    [{ path: '/new', Component: NewBlueprint as never }],
    { initialEntries: ['/new'], auth: options?.auth },
  );
}

/** Override the GET /api/v1/agents/:account/my-agent endpoint for the duration of one test. */
function overrideBlueprintGet(blueprint: Blueprint | null) {
  server.use(
    http.get('/api/v1/agents/:account/:name', ({ params }) => {
      if (params.account !== ACCOUNT || params.name !== NAME) {
        return HttpResponse.json({ error: 'not_found' }, { status: 404 });
      }
      if (!blueprint) {
        return HttpResponse.json({ error: 'not_found' }, { status: 404 });
      }
      return HttpResponse.json(blueprint);
    }),
  );
}

const base: Blueprint = { name: NAME, account: ACCOUNT, registry: '', versions: [] };

async function typeNameAndWait(name: string) {
  const user = userEvent.setup();
  const input = screen.getByPlaceholderText('my-agent');
  await user.type(input, name);
}

// ── Name availability UI ──────────────────────────────────────────────────────

describe('NewBlueprint – name availability UI', () => {
  it('shows "Will be created as" when the name does not exist', async () => {
    // Default handler returns 404 for names not in mockBlueprints (e.g. "my-agent").
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/will be created as/i)).toBeInTheDocument();
    });
  });

  it('shows "already exists" when an active blueprint has that name', async () => {
    overrideBlueprintGet(base); // active: no archived_at, name_reserved irrelevant
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/already exists/i)).toBeInTheDocument();
    });
  });

  it('shows "Will be created as" when the blueprint is archived and name is not reserved', async () => {
    overrideBlueprintGet({ ...base, archived_at: '2025-01-01T00:00:00Z', name_reserved: false });
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/will be created as/i)).toBeInTheDocument();
    });
  });

  it('shows "already exists" when the blueprint is archived but name is reserved', async () => {
    overrideBlueprintGet({ ...base, archived_at: '2025-01-01T00:00:00Z', name_reserved: true });
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/already exists/i)).toBeInTheDocument();
    });
  });
});

// ── Create button disabled state ──────────────────────────────────────────────

describe('NewBlueprint – Create button disabled state', () => {
  it('disables "Continue" when the name is taken', async () => {
    overrideBlueprintGet(base);
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/already exists/i)).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /^continue$/i })).toBeDisabled();
  });

  it('enables "Continue" when the name is available', async () => {
    // Default handler: 404 → available
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/will be created as/i)).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled();
  });
});

describe('NewBlueprint – org scoping', () => {
  it('calls switchOrg with org organization_id when session is not yet scoped to the org', async () => {
    const switchOrg = vi.fn(async () => {});
    const auth: AuthContextType = {
      ...mockAuthContext,
      organizationId: null,
      accounts: [
        { id: 'acct-1', name: 'testuser', type: 'personal' },
        { id: 'acct-2', name: 'my-org', type: 'organization', organization_id: 'org-id-2' },
      ],
      switchOrg,
    };

    server.use(
      http.post('/api/v1/agents/:account', ({ params }) => {
        return HttpResponse.json({ account: params.account, name: 'test-agent' });
      }),
    );

    renderNewBlueprint({ auth });

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('my-agent'), 'test-agent');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled();
    });

    await user.click(screen.getByRole('combobox'));
    await waitFor(() => screen.getByRole('option', { name: /my-org/i }));
    await user.click(screen.getByRole('option', { name: /my-org/i }));

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByText('Set up locally'));
    await user.click(screen.getByRole('button', { name: /create blueprint/i }));

    await waitFor(() => {
      expect(switchOrg).toHaveBeenCalledWith('org-id-2');
    });
  });

  it('does not call switchOrg when the session is already scoped to the selected org', async () => {
    const switchOrg = vi.fn(async () => {});
    const createCalled = vi.fn();
    const auth: AuthContextType = {
      ...mockAuthContext,
      organizationId: 'org-id-2',
      accounts: [
        { id: 'acct-1', name: 'testuser', type: 'personal' },
        { id: 'acct-2', name: 'my-org', type: 'organization', organization_id: 'org-id-2' },
      ],
      switchOrg,
    };

    server.use(
      http.post('/api/v1/agents/:account', ({ params }) => {
        createCalled();
        return HttpResponse.json({ account: params.account, name: 'test-agent' });
      }),
    );

    renderNewBlueprint({ auth });

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('my-agent'), 'test-agent');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled();
    });

    await user.click(screen.getByRole('combobox'));
    await waitFor(() => screen.getByRole('option', { name: /my-org/i }));
    await user.click(screen.getByRole('option', { name: /my-org/i }));

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByText('Set up locally'));
    await user.click(screen.getByRole('button', { name: /create blueprint/i }));

    await waitFor(() => expect(createCalled).toHaveBeenCalled());
    expect(switchOrg).not.toHaveBeenCalled();
  });
});
