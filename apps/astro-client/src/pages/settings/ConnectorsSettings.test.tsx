import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderWithProviders } from '@/test/test-utils';
import ConnectorsSettings from './ConnectorsSettings';

afterEach(cleanup);

const ACCOUNT = 'testuser';

function mockGitHub(opts?: {
  status?: { connected: boolean; github_login?: string };
  orgs?: Array<{ login: string; avatar_url: string }>;
}) {
  const status = opts?.status ?? { connected: false };
  const orgs = opts?.orgs ?? [];
  server.use(
    http.get(`/api/v1/accounts/${ACCOUNT}/github`, () => HttpResponse.json(status)),
    http.get(`/api/v1/accounts/${ACCOUNT}/github/orgs`, () => HttpResponse.json({ orgs })),
  );
}

function mockSlack(workspaces: Array<{
  team_id: string;
  slack_user_id: string;
  team?: string;
  team_domain?: string;
  slack_username?: string;
}> = []) {
  server.use(
    http.get(`/api/v1/accounts/${ACCOUNT}/slack`, () => HttpResponse.json({ workspaces })),
  );
}

describe('ConnectorsSettings', () => {
  it('renders the shared "Connectors" header', async () => {
    mockGitHub();
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    expect(screen.getByRole('heading', { name: 'Connectors' })).toBeInTheDocument();
    expect(
      screen.getByText('Connect your Astro account to external services'),
    ).toBeInTheDocument();
  });

  it('shows Connect buttons with descriptive aria-labels when both connectors are disconnected', async () => {
    mockGitHub();
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Connect GitHub' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Connect Slack' })).toBeInTheDocument();
    });

    expect(
      screen.getByText('Build and deploy agents directly from your repositories.'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Message agents directly from any connected Slack workspace.'),
    ).toBeInTheDocument();
  });

  it('shows the GitHub login + org count and kebab menu when connected', async () => {
    mockGitHub({
      status: { connected: true, github_login: 'octocat' },
      orgs: [
        { login: 'acme', avatar_url: '' },
        { login: 'globex', avatar_url: '' },
      ],
    });
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    expect(await screen.findByText('@octocat')).toBeInTheDocument();
    expect(await screen.findByText('acme')).toBeInTheDocument();

    expect(
      screen.getByText(
        (_, node) => node?.textContent === 'Connected as @octocat · 2 organizations',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'GitHub options' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect GitHub' })).not.toBeInTheDocument();

    const list = screen.getAllByRole('list')[0];
    const orgItems = within(list).getAllByRole('listitem');
    expect(orgItems[0]).toHaveTextContent('acme');
    expect(orgItems[1]).toHaveTextContent('globex');
  });

  it('shows the "Request access" empty-state row when GitHub is connected with no orgs', async () => {
    mockGitHub({ status: { connected: true, github_login: 'octocat' }, orgs: [] });
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    const links = await screen.findAllByRole('link', { name: 'Request access' });
    expect(links[0]).toHaveAttribute(
      'href',
      'https://github.com/settings/connections/applications',
    );
    expect(links[0].closest('li')).toHaveTextContent(
      'No organizations have approved Astro yet. Request access on GitHub.',
    );
  });

  it('opens the GitHub disconnect confirmation when the kebab "Disconnect" item is clicked', async () => {
    const user = userEvent.setup();
    mockGitHub({ status: { connected: true, github_login: 'octocat' }, orgs: [] });
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    await user.click(await screen.findByRole('button', { name: 'GitHub options' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Disconnect' }));

    expect(
      await screen.findByRole('heading', { name: 'Disconnect GitHub?' }),
    ).toBeInTheDocument();
  });

  it('renders Slack workspaces with a per-row icon disconnect button and no copy-user-id button', async () => {
    mockGitHub();
    mockSlack([
      {
        team_id: 'T1',
        slack_user_id: 'U1',
        team: 'Acme Slack',
        slack_username: 'octocat',
      },
    ]);
    renderWithProviders(<ConnectorsSettings />);

    await waitFor(() => {
      expect(screen.getByText('Acme Slack')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: 'Disconnect Acme Slack' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /copy.*user id/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Remove' })).not.toBeInTheDocument();
    expect(screen.getByText('@octocat')).toBeInTheDocument();
    expect(screen.getByText(/1 workspace$/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add workspace' })).toBeInTheDocument();
  });

  it('opens the per-workspace remove dialog when the icon disconnect button is clicked', async () => {
    const user = userEvent.setup();
    mockGitHub();
    mockSlack([
      { team_id: 'T1', slack_user_id: 'U1', team: 'Acme Slack' },
    ]);
    renderWithProviders(<ConnectorsSettings />);

    await user.click(
      await screen.findByRole('button', { name: 'Disconnect Acme Slack' }),
    );

    expect(
      await screen.findByRole('heading', { name: 'Remove Slack workspace?' }),
    ).toBeInTheDocument();
  });

  it('pluralizes the workspace count for multiple Slack workspaces', async () => {
    mockGitHub();
    mockSlack([
      { team_id: 'T1', slack_user_id: 'U1', team: 'Acme Slack' },
      { team_id: 'T2', slack_user_id: 'U1', team: 'Globex Slack' },
    ]);
    renderWithProviders(<ConnectorsSettings />);

    await waitFor(() => {
      expect(screen.getByText(/2 workspaces$/)).toBeInTheDocument();
    });
  });
});
