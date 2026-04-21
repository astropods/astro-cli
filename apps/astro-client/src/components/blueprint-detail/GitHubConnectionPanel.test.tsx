import { screen, waitFor, cleanup, fireEvent } from '@testing-library/react';
import { describe, it, expect, afterEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { renderWithProviders } from '@/test/test-utils';
import { server } from '@/test/msw/server';
import { GitHubConnectionPanel } from './GitHubConnectionPanel';
import type { GitHubRepo, GitHubStatusResponse } from '@/lib/api';

afterEach(cleanup);

const REPOS: GitHubRepo[] = [
  { full_name: 'testuser/my-agent', default_branch: 'main', private: false },
  { full_name: 'testuser/private-repo', default_branch: 'main', private: true },
];

const notConnected: GitHubStatusResponse = { connected: false, builds: [] };

function useMocks() {
  server.use(
    http.get('/api/v1/agents/testuser/my-agent/github', () => HttpResponse.json(notConnected)),
    http.get('/api/v1/accounts/testuser/github/repos', () =>
      HttpResponse.json({ repos: REPOS, has_more: false })
    ),
  );
}

function render() {
  return renderWithProviders(
    <GitHubConnectionPanel account="testuser" name="my-agent" />,
    { initialEntries: ['/?github_connected=true'] },
  );
}

describe('GitHubConnectionPanel – RepoSelectorDialog', () => {
  it('opens dialog and shows search input when github_connected=true', async () => {
    useMocks();
    render();

    await waitFor(() => {
      expect(screen.getByText('Connect GitHub repository')).toBeInTheDocument();
    });
    expect(screen.getByPlaceholderText(/search repositories/i)).toBeInTheDocument();
  });

  it('lists repos from the account-level endpoint after opening the dropdown', async () => {
    useMocks();
    render();

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search repositories/i)).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText(/search repositories/i), {
      target: { value: 'a' },
    });

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /my-agent/ })).toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: /private-repo/ })).toBeInTheDocument();
  });

  it('does not call the old per-agent repos endpoint', async () => {
    let perAgentCalled = false;
    server.use(
      http.get('/api/v1/agents/testuser/my-agent/github', () => HttpResponse.json(notConnected)),
      http.get('/api/v1/accounts/testuser/github/repos', () =>
        HttpResponse.json({ repos: REPOS, has_more: false })
      ),
      http.get('/api/v1/agents/testuser/my-agent/github/repos', () => {
        perAgentCalled = true;
        return HttpResponse.json({ repos: [] });
      }),
    );

    render();

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search repositories/i)).toBeInTheDocument();
    });

    expect(perAgentCalled).toBe(false);
  });
});
