import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
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
});
