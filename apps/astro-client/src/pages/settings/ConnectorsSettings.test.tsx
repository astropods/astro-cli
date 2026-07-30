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
  icon?: string;
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
      screen.getByText('Connect external services to use them across Astro'),
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

  it('shows the GitHub login and kebab menu when connected', async () => {
    mockGitHub({
      status: { connected: true, github_login: 'octocat' },
      orgs: [
        { login: 'acme', avatar_url: 'https://example.com/acme.png' },
        { login: 'globex', avatar_url: '' },
      ],
    });
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    await screen.findByText('@octocat');
    expect(screen.getByText('Connected as').parentElement).toHaveTextContent(
      'Connected as @octocat',
    );
    expect(screen.queryByText('Account')).not.toBeInTheDocument();
    expect(await screen.findByText('acme')).toBeInTheDocument();

    expect(screen.queryByText('Connected')).not.toBeInTheDocument();
    expect(screen.queryByText(/2 organizations/)).not.toBeInTheDocument();
    const optionsButton = screen.getByRole('button', { name: 'GitHub options' });
    expect(optionsButton).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect GitHub' })).not.toBeInTheDocument();

    const list = screen.getAllByRole('list')[0];
    const orgItems = within(list).getAllByRole('listitem');
    expect(orgItems).toHaveLength(2);
    expect(orgItems[0]).toHaveTextContent('acme');
    expect(orgItems[0].querySelector('img')).toHaveAttribute(
      'src',
      'https://example.com/acme.png',
    );
    expect(orgItems[1]).toHaveTextContent('globex');
    expect(orgItems[1].querySelector('img')).toBeNull();
    expect(orgItems[1].querySelector('svg')).toBeInTheDocument();
    const missingPrompt = screen.getByText('Missing an organization?');
    expect(missingPrompt).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Request access on GitHub' }),
    ).toHaveAttribute(
      'href',
      'https://github.com/settings/connections/applications',
    );
  });

  it('shows GitHub access guidance when connected with no orgs', async () => {
    mockGitHub({ status: { connected: true, github_login: 'octocat' }, orgs: [] });
    mockSlack();
    renderWithProviders(<ConnectorsSettings />);

    const prompt = await screen.findByText(
      'No organizations have approved Astro yet.',
    );
    expect(prompt).toBeInTheDocument();
    const link = screen.getByRole('link', { name: 'Request access on GitHub' });
    expect(link).toHaveAttribute(
      'href',
      'https://github.com/settings/connections/applications',
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

  it('renders Slack workspace identity and controls', async () => {
    mockGitHub();
    mockSlack([
      {
        team_id: 'T1',
        slack_user_id: 'U1',
        team: 'Acme Slack',
        slack_username: 'octocat',
        icon: 'https://example.com/acme-slack.png',
      },
    ]);
    renderWithProviders(<ConnectorsSettings />);

    await waitFor(() => {
      expect(screen.getByText('Acme Slack')).toBeInTheDocument();
    });

    expect(screen.getByText('Acme Slack').closest('li')?.querySelector('img')).toHaveAttribute(
      'src',
      'https://example.com/acme-slack.png',
    );
    const disconnectButton = screen.getByRole('button', { name: 'Disconnect Acme Slack' });
    expect(disconnectButton).toHaveTextContent('Disconnect');
    expect(screen.queryByRole('button', { name: /copy.*user id/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Remove' })).not.toBeInTheDocument();
    expect(screen.getByText('@octocat')).toBeInTheDocument();
    expect(screen.queryByText('Connected')).not.toBeInTheDocument();
    expect(screen.getByText('Connected to 1 workspace')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add workspace' })).toBeEnabled();
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

  it('shows the Slack workspace count beneath the connector name', async () => {
    mockGitHub();
    mockSlack([
      {
        team_id: 'T1',
        slack_user_id: 'U1',
        team: 'Acme Slack',
        slack_username: 'octocat',
      },
      {
        team_id: 'T2',
        slack_user_id: 'U1',
        team: 'Globex Slack',
        slack_username: 'globex-admin',
      },
    ]);
    renderWithProviders(<ConnectorsSettings />);

    expect(await screen.findByText('Connected to 2 workspaces')).toBeInTheDocument();
    expect(screen.queryByText('Connected')).not.toBeInTheDocument();
    expect(screen.getByText('@octocat')).toBeInTheDocument();
    expect(screen.getByText('@globex-admin')).toBeInTheDocument();
  });

  it('omits the Slack identity tag when a workspace has no username', async () => {
    mockGitHub();
    mockSlack([
      { team_id: 'T1', slack_user_id: 'U1', team: 'Acme Slack' },
    ]);
    renderWithProviders(<ConnectorsSettings />);

    expect(await screen.findByText('Acme Slack')).toBeInTheDocument();
    expect(screen.getByText('Acme Slack').closest('li')?.querySelector('img')).toBeNull();
    expect(screen.getByText('Acme Slack').closest('li')?.querySelector('svg')).toBeInTheDocument();
    expect(screen.queryByText(/^@/)).not.toBeInTheDocument();
  });
});
