import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute } from '@/test/test-utils';
import type { Blueprint } from '@/lib/api';
import NewBlueprint from './NewBlueprint';

afterEach(cleanup);

const ACCOUNT = 'testuser';
const NAME = 'my-agent';

function renderNewBlueprint() {
  return renderRoute(
    [{ path: '/new', Component: NewBlueprint as never }],
    { initialEntries: ['/new'] },
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
  it('disables "Create blueprint" when the name is taken', async () => {
    overrideBlueprintGet(base);
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/already exists/i)).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /create blueprint/i })).toBeDisabled();
  });

  it('enables "Create blueprint" when the name is available', async () => {
    // Default handler: 404 → available
    renderNewBlueprint();
    await typeNameAndWait(NAME);

    await waitFor(() => {
      expect(screen.getByText(/will be created as/i)).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /create blueprint/i })).not.toBeDisabled();
  });
});
