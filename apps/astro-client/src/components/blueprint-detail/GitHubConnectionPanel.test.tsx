import { screen, waitFor, cleanup, fireEvent } from '@testing-library/react';
import { describe, it, expect, afterEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { renderWithProviders } from '@/test/test-utils';
import { server } from '@/test/msw/server';
import { GitHubConnectionPanel } from './GitHubConnectionPanel';
import type { GitHubBuild, GitHubRepo, GitHubStatusResponse } from '@/lib/api';

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

describe('GitHubConnectionPanel – connected account, no repo linked', () => {
  function useMocksAccountConnectedNoRepo() {
    server.use(
      http.get('/api/v1/agents/testuser/my-agent/github', () => HttpResponse.json(notConnected)),
      http.get('/api/v1/accounts/testuser/github', () =>
        HttpResponse.json({ connected: true, github_login: 'gh-user' })
      ),
      http.get('/api/v1/accounts/testuser/github/repos', () =>
        HttpResponse.json({ repos: REPOS, has_more: false })
      ),
    );
  }

  function renderNoParam() {
    return renderWithProviders(
      <GitHubConnectionPanel account="testuser" name="my-agent" />,
      { initialEntries: ['/'] },
    );
  }

  it('renders without crashing', async () => {
    useMocksAccountConnectedNoRepo();
    renderNoParam();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /connect github repo/i })).toBeInTheDocument();
    });
  });

  it('shows the "Connect GitHub repo" button when no repo is linked', async () => {
    useMocksAccountConnectedNoRepo();
    renderNoParam();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /connect github repo/i })).toBeInTheDocument();
    });
  });

  it('does NOT show repo picker dialog when github_connected param is absent', async () => {
    useMocksAccountConnectedNoRepo();
    renderNoParam();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /connect github repo/i })).toBeInTheDocument();
    });

    expect(screen.queryByText('Connect GitHub repository')).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/search repositories/i)).not.toBeInTheDocument();
  });
});

describe('GitHubConnectionPanel – build rows', () => {
  function makeBuild(overrides: Partial<GitHubBuild>): GitHubBuild {
    return {
      id: '1',
      build_id: 'bld-abc',
      commit_sha: 'abc1234',
      branch: 'main',
      status: 'registered',
      enqueued_at: '2024-01-01T00:00:00Z',
      completed_at: '2024-01-01T00:01:00Z',
      ...overrides,
    };
  }

  function renderWithBuild(build: GitHubBuild) {
    const connected: GitHubStatusResponse = {
      connected: true,
      repo_full_name: 'testuser/my-agent',
      branch: 'main',
      builds: [build],
    };
    server.use(
      http.get('/api/v1/agents/testuser/my-agent/github', () => HttpResponse.json(connected)),
    );
    return renderWithProviders(
      <GitHubConnectionPanel account="testuser" name="my-agent" />,
      { initialEntries: ['/'] },
    );
  }

  it.each([
    { status: 'skipped'    as const, expectedText: /skipped.*no changes detected/i },
    { status: 'registered' as const, expectedText: /successful/i },
    { status: 'failed'     as const, expectedText: /error.*see logs for more/i },
  ])('shows correct label for $status build', async ({ status, expectedText }) => {
    renderWithBuild(makeBuild({ status, commit_message: 'test commit' }));

    await waitFor(() => {
      expect(screen.getByText(expectedText)).toBeInTheDocument();
    });
  });
});

describe('GitHubConnectionPanel – direct connect (no OAuth)', () => {
  function useMocksDirectConnect() {
    server.use(
      http.get('/api/v1/agents/testuser/my-agent/github', () => HttpResponse.json(notConnected)),
      http.get('/api/v1/accounts/testuser/github', () =>
        HttpResponse.json({ connected: false })
      ),
      http.post('/api/v1/accounts/testuser/github/connect', () =>
        HttpResponse.json({ connected: true, github_login: 'gh-user' })
      ),
      http.get('/api/v1/accounts/testuser/github/repos', () =>
        HttpResponse.json({ repos: REPOS, has_more: false })
      ),
    );
  }

  function renderNoParam() {
    return renderWithProviders(
      <GitHubConnectionPanel account="testuser" name="my-agent" />,
      { initialEntries: ['/'] },
    );
  }

  it('clicking "Connect GitHub repo" calls connect and opens the repo picker dialog', async () => {
    useMocksDirectConnect();
    renderNoParam();

    const connectBtn = await screen.findByRole('button', { name: /connect github repo/i });
    fireEvent.click(connectBtn);

    await waitFor(() => {
      expect(screen.getByText('Connect GitHub repository')).toBeInTheDocument();
    });
    expect(screen.getByPlaceholderText(/search repositories/i)).toBeInTheDocument();
  });
});
