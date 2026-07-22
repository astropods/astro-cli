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

// ── Continue submit gate ──────────────────────────────────────────────────────

describe('NewBlueprint – Continue submit gate', () => {
  it('blocks a blank name on submit instead of disabling the button', async () => {
    renderNewBlueprint();
    const user = userEvent.setup();

    const continueBtn = screen.getByRole('button', { name: /^continue$/i });
    expect(continueBtn).not.toBeDisabled();

    await user.click(continueBtn);

    expect(await screen.findByText(/name is required/i)).toBeInTheDocument();
    expect(screen.queryByText('Set up locally')).not.toBeInTheDocument();
  });

  it('blocks a too-short name on submit and does not advance', async () => {
    renderNewBlueprint();
    const user = userEvent.setup();
    await typeNameAndWait('ab');

    await user.click(screen.getByRole('button', { name: /^continue$/i }));

    expect(await screen.findByText(/at least 4 characters/i)).toBeInTheDocument();
    expect(screen.queryByText('Set up locally')).not.toBeInTheDocument();
  });

  it('does not advance when the name is taken', async () => {
    overrideBlueprintGet(base);
    renderNewBlueprint();
    const user = userEvent.setup();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/already exists/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    expect(screen.queryByText('Set up locally')).not.toBeInTheDocument();
  });

  it('advances to the source step when the name is available', async () => {
    // Default handler: 404 → available
    renderNewBlueprint();
    const user = userEvent.setup();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/will be created as/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    expect(await screen.findByText('Set up locally')).toBeInTheDocument();
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

describe('NewBlueprint – blueprint-limit quota link', () => {
  const LIMIT_BODY = {
    error: 'Limit reached',
    code: 'ENTITLEMENT_LIMIT_REACHED',
    feature: 'blueprints',
    usage: 5,
    limit: 5,
    details:
      'Blueprints limit reached (5 of 5 used): Your account has reached the maximum number of registered blueprints; archive unused blueprints to free capacity. To continue, request a quota increase from Settings > Usage.',
  };

  it('links the blueprint-limit error to the org-scoped Settings → Usage page', async () => {
    const auth: AuthContextType = {
      ...mockAuthContext,
      organizationId: 'org-id-2',
      accounts: [
        { id: 'acct-1', name: 'testuser', type: 'personal' },
        { id: 'acct-2', name: 'my-org', type: 'organization', organization_id: 'org-id-2' },
      ],
    };
    server.use(
      http.post('/api/v1/agents/:account', () => HttpResponse.json(LIMIT_BODY, { status: 402 })),
    );

    renderNewBlueprint({ auth });

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('my-agent'), 'test-agent');
    await waitFor(() => expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled());

    await user.click(screen.getByRole('combobox'));
    await waitFor(() => screen.getByRole('option', { name: /my-org/i }));
    await user.click(screen.getByRole('option', { name: /my-org/i }));

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByText('Set up locally'));
    await user.click(screen.getByRole('button', { name: /create blueprint/i }));

    const link = await screen.findByRole('link', { name: /review your billing in settings/i });
    expect(link).toHaveAttribute('href', '/settings/org/my-org/billing');
  });

  it('links to the personal Settings → Usage page for a personal account', async () => {
    server.use(
      http.post('/api/v1/agents/:account', () => HttpResponse.json(LIMIT_BODY, { status: 402 })),
    );

    renderNewBlueprint();

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('my-agent'), 'test-agent');
    await waitFor(() => expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled());

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByText('Set up locally'));
    await user.click(screen.getByRole('button', { name: /create blueprint/i }));

    const link = await screen.findByRole('link', { name: /review your billing in settings/i });
    expect(link).toHaveAttribute('href', '/settings/billing');
  });

  it('opens the request-quota-increase dialog from the blueprint-limit message', async () => {
    server.use(
      http.post('/api/v1/agents/:account', () => HttpResponse.json(LIMIT_BODY, { status: 402 })),
    );

    renderNewBlueprint();

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('my-agent'), 'test-agent');
    await waitFor(() => expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled());

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByText('Set up locally'));
    await user.click(screen.getByRole('button', { name: /create blueprint/i }));

    await user.click(await screen.findByRole('button', { name: /request a quota increase/i }));
    expect(await screen.findByRole('dialog')).toHaveTextContent(/request quota increase/i);
  });
});

describe('NewBlueprint – GitHub already connected', () => {
  it('skips the Connect GitHub step and shows the repo list when already connected', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/github', () =>
        HttpResponse.json({ connected: true, github_login: 'gh-user' })),
      // RepoPicker mounts once connected — satisfy its data requests.
      http.get('/api/v1/accounts/:account/github/repos', () =>
        HttpResponse.json({ repos: [], has_more: false })),
      http.get('/api/v1/accounts/:account/github/connections', () =>
        HttpResponse.json({ connections: [] })),
    );

    renderNewBlueprint();

    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('my-agent'), 'test-agent');
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^continue$/i })).not.toBeDisabled();
    });

    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByText('Set up with GitHub'));

    // Already connected: the repo list is revealed directly, with no extra
    // "Connect GitHub" click required.
    expect(await screen.findByText('gh-user connected')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /connect github/i })).not.toBeInTheDocument();
  });
});
