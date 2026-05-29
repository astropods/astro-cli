import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderWithProviders } from '@/test/test-utils';
import { VaultSettings } from './SecretsSettings';

afterEach(cleanup);

describe('VaultSettings', () => {
  it('shows ErrorPanel when variables API returns insufficient permissions', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/variables', () =>
        HttpResponse.json({ error: 'insufficient permissions for this account' }, { status: 403 }),
      ),
    );

    renderWithProviders(<VaultSettings account="myorg" />);

    await waitFor(() => {
      expect(screen.getByText('Could not load variables')).toBeInTheDocument();
      expect(screen.getByText('insufficient permissions for this account')).toBeInTheDocument();
    });
  });

  it('shows ErrorPanel when variables API returns switch-org scope error', async () => {
    const msg = 'session is not scoped to this organization, use switch-org first';
    server.use(
      http.get('/api/v1/accounts/:account/variables', () =>
        HttpResponse.json({ error: msg }, { status: 403 }),
      ),
    );

    renderWithProviders(<VaultSettings account="myorg" />);

    await waitFor(() => {
      expect(screen.getByText('Could not load variables')).toBeInTheDocument();
      expect(screen.getByText(msg)).toBeInTheDocument();
    });
  });

  it('shows empty vault state when list succeeds with no variables (not an error panel)', async () => {
    renderWithProviders(<VaultSettings account="myorg" />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    expect(screen.getByText('No variables yet')).toBeInTheDocument();
    expect(screen.queryByText('Could not load variables')).not.toBeInTheDocument();
  });

  it('does not show a reveal toggle for stored secrets in the vault table', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/variables', () =>
        HttpResponse.json({
          variables: [
            {
              name: 'API_KEY',
              secret: true,
              description: 'Primary key',
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-02T00:00:00Z',
            },
          ],
        }),
      ),
    );

    renderWithProviders(<VaultSettings account="myorg" />);

    await waitFor(() => {
      expect(screen.getByText('API_KEY')).toBeInTheDocument();
    });

    expect(screen.getByText('••••••••')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /reveal value/i })).not.toBeInTheDocument();
  });

  it('does not show a reveal toggle when editing an existing secret', async () => {
    const user = userEvent.setup()

    server.use(
      http.get('/api/v1/accounts/:account/variables', () =>
        HttpResponse.json({
          variables: [
            {
              name: 'API_KEY',
              secret: true,
              description: 'Primary key',
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-02T00:00:00Z',
            },
          ],
        }),
      ),
    );

    renderWithProviders(<VaultSettings account="myorg" />);

    await waitFor(() => {
      expect(screen.getByText('API_KEY')).toBeInTheDocument();
    });

    const row = screen.getByRole('row', { name: /API_KEY/i });
    await user.click(within(row).getByRole('button'));
    await user.click(screen.getByRole('menuitem', { name: 'Edit' }));

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /reveal value/i })).not.toBeInTheDocument();
  });

  it('updates a secret description without sending a new value', async () => {
    const user = userEvent.setup();
    let putBody: Record<string, unknown> | undefined;

    server.use(
      http.get('/api/v1/accounts/:account/variables', () =>
        HttpResponse.json({
          variables: [
            {
              name: 'API_KEY',
              secret: true,
              description: 'Primary key',
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-02T00:00:00Z',
            },
          ],
        }),
      ),
      http.put('/api/v1/accounts/:account/variables/:varName', async ({ request }) => {
        putBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({
          name: 'API_KEY',
          secret: true,
          description: putBody.description,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-03T00:00:00Z',
        });
      }),
    );

    renderWithProviders(<VaultSettings account="myorg" />);

    await waitFor(() => {
      expect(screen.getByText('API_KEY')).toBeInTheDocument();
    });

    const row = screen.getByRole('row', { name: /API_KEY/i });
    await user.click(within(row).getByRole('button'));
    await user.click(screen.getByRole('menuitem', { name: 'Edit' }));

    await user.clear(screen.getByLabelText(/description/i));
    await user.type(screen.getByLabelText(/description/i), 'Updated description');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(putBody).toEqual({ description: 'Updated description' });
    });
  });
});
