import { describe, it, expect, afterEach, vi, beforeEach } from 'vitest';
import { screen, cleanup, fireEvent } from '@testing-library/react';
import { renderWithProviders } from '@/test/test-utils';
import { DeployedAgentCard } from './DeployedAgentCard';

const mockNavigate = vi.fn();
vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router')>();
  return { ...actual, useNavigate: () => mockNavigate };
});

beforeEach(() => mockNavigate.mockClear());
afterEach(cleanup);

const baseProps = {
  name: 'code-reviewer',
  deploymentId: 'dep-123',
  account: 'testuser',
  href: '/testuser/code-reviewer',
  status: 'active' as const,
  requests: 42,
  lastActive: '2 hours ago',
  installedAt: 'Apr 1, 2025',
  updatedAt: 'Apr 1, 2025',
};

describe('DeployedAgentCard', () => {
  it('renders displayName when provided', () => {
    renderWithProviders(
      <DeployedAgentCard {...baseProps} displayName="My Code Reviewer" />,
    );

    expect(screen.getByText('My Code Reviewer')).toBeInTheDocument();
    expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
  });

  it('falls back to name when displayName is not provided', () => {
    renderWithProviders(<DeployedAgentCard {...baseProps} />);

    expect(screen.getByText('code-reviewer')).toBeInTheDocument();
  });

  it('falls back to name when displayName is empty string', () => {
    renderWithProviders(
      <DeployedAgentCard {...baseProps} displayName="" />,
    );

    expect(screen.getByText('code-reviewer')).toBeInTheDocument();
  });

  // Phase 3 — upgrade affordance lit by server-supplied latest_build_id

  it('renders Update available badge when hasNewBuildAvailable is true', () => {
    renderWithProviders(
      <DeployedAgentCard
        {...baseProps}
        hasNewBuildAvailable
        latestBuildId="build-new"
        currentBuildId="build-old"
      />,
    );

    expect(screen.getByLabelText('Upgrade to newest build')).toBeInTheDocument();
    expect(screen.getByText('Update available')).toBeInTheDocument();
  });

  it('does not render Update badge when hasNewBuildAvailable is false', () => {
    renderWithProviders(<DeployedAgentCard {...baseProps} />);

    expect(screen.queryByText('Update available')).not.toBeInTheDocument();
  });

  // Without a latestBuildId the badge has nothing to upgrade to,
  // so it falls back to the static Tag — clicking shouldn't pop the modal.
  it('renders non-clickable badge when latestBuildId is missing', () => {
    renderWithProviders(
      <DeployedAgentCard
        {...baseProps}
        hasNewBuildAvailable
        currentBuildId="build-old"
      />,
    );

    expect(screen.queryByLabelText('Upgrade to newest build')).not.toBeInTheDocument();
    expect(screen.getByText('Update available')).toBeInTheDocument();
  });

  it('opens the upgrade confirm dialog when the badge is clicked', () => {
    renderWithProviders(
      <DeployedAgentCard
        {...baseProps}
        hasNewBuildAvailable
        latestBuildId="build-new12345"
        currentBuildId="build-old12345"
      />,
    );

    fireEvent.click(screen.getByLabelText('Upgrade to newest build'));
    expect(screen.getByText('Upgrade to newest build?')).toBeInTheDocument();
    // Truncated build IDs (first 8 chars) appear in the description.
    expect(screen.getByText(/build-ne/)).toBeInTheDocument();
    expect(screen.getByText(/build-ol/)).toBeInTheDocument();
  });

  it('navigates to configure?build=<latestBuildId> when upgrade is confirmed', () => {
    renderWithProviders(
      <DeployedAgentCard
        {...baseProps}
        hasNewBuildAvailable
        latestBuildId="build-new12345"
        currentBuildId="build-old12345"
      />,
    );

    fireEvent.click(screen.getByLabelText('Upgrade to newest build'));
    fireEvent.click(screen.getByRole('button', { name: /continue/i }));

    expect(mockNavigate).toHaveBeenCalledWith(
      '/testuser/agents/dep-123/configure?build=build-new12345',
    );
  });

  it('renders error_message as accessible label on the status badge when status=error', () => {
    renderWithProviders(
      <DeployedAgentCard
        {...baseProps}
        status="error"
        errorMessage="ImagePullBackOff on pod agent-abc"
      />,
    );

    expect(screen.getByLabelText('ImagePullBackOff on pod agent-abc')).toBeInTheDocument();
  });
});
