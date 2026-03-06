import { describe, it, expect, afterEach } from 'vitest';
import { screen, cleanup } from '@testing-library/react';
import { renderWithProviders } from '@/test/test-utils';
import { DeployedAgentCard } from './DeployedAgentCard';

afterEach(cleanup);

const baseProps = {
  name: 'code-reviewer',
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
});
