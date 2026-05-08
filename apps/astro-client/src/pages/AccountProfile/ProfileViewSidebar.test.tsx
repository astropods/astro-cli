import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { ProfileViewSidebar } from './ProfileViewSidebar';
import type { AccountPublic } from '@/lib/api';

afterEach(cleanup);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const baseAccount: AccountPublic = {
  id: 'acct-1',
  name: 'testuser',
  type: 'personal',
  display_name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

const defaultProps = {
  data: baseAccount,
  isOwner: false,
  blueprintCount: 0,
  deploymentCount: 0,
  orgs: [],
};

// ── Display name ──────────────────────────────────────────────────────────────

describe('ProfileViewSidebar display name', () => {
  it('renders display_name when set', () => {
    renderWithProviders(<ProfileViewSidebar {...defaultProps} />);
    expect(screen.getByRole('heading', { name: 'Test User' })).toBeInTheDocument();
  });

  it('falls back to account name when display_name is absent', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...defaultProps}
        data={{ ...baseAccount, display_name: undefined }}
      />,
    );
    expect(screen.getByRole('heading', { name: 'testuser' })).toBeInTheDocument();
  });

  it('renders the @handle below the display name', () => {
    renderWithProviders(<ProfileViewSidebar {...defaultProps} />);
    expect(screen.getByText('@testuser')).toBeInTheDocument();
  });
});

// ── Edit profile button ───────────────────────────────────────────────────────

describe('ProfileViewSidebar edit profile button', () => {
  it('shows the edit button when isOwner and onEditOpen are both provided', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} isOwner onEditOpen={vi.fn()} />,
    );
    expect(screen.getByRole('button', { name: /edit profile/i })).toBeInTheDocument();
  });

  it('hides the edit button when isOwner is false', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} isOwner={false} onEditOpen={vi.fn()} />,
    );
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });

  it('hides the edit button when onEditOpen is undefined (external view)', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} isOwner onEditOpen={undefined} />,
    );
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });

  it('calls onEditOpen when the edit button is clicked', async () => {
    const user = userEvent.setup();
    const onEditOpen = vi.fn();
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} isOwner onEditOpen={onEditOpen} />,
    );
    await user.click(screen.getByRole('button', { name: /edit profile/i }));
    expect(onEditOpen).toHaveBeenCalledOnce();
  });
});

// ── Stats ─────────────────────────────────────────────────────────────────────

describe('ProfileViewSidebar stats', () => {
  it('shows the Blueprints stat', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} blueprintCount={5} />,
    );
    // Stat label is uppercase via CSS; DOM text is "Blueprints"
    expect(screen.getByText('Blueprints')).toBeInTheDocument();
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('shows the Agents stat for the owner', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} isOwner deploymentCount={3} />,
    );
    expect(screen.getByText('Agents')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('hides the Agents stat for a visitor', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} isOwner={false} deploymentCount={3} />,
    );
    expect(screen.queryByText('Agents')).not.toBeInTheDocument();
  });
});

// ── Optional meta fields ──────────────────────────────────────────────────────

describe('ProfileViewSidebar optional meta fields', () => {
  it('renders bio when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...defaultProps}
        data={{ ...baseAccount, bio: 'I build things.' }}
      />,
    );
    expect(screen.getByText('I build things.')).toBeInTheDocument();
  });

  it('renders pronouns when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...defaultProps}
        data={{ ...baseAccount, pronouns: 'they/them' }}
      />,
    );
    expect(screen.getByText('they/them')).toBeInTheDocument();
  });

  it('renders location when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...defaultProps}
        data={{ ...baseAccount, location: 'San Francisco' }}
      />,
    );
    expect(screen.getByText('San Francisco')).toBeInTheDocument();
  });

  it('renders email as a mailto link', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...defaultProps}
        data={{ ...baseAccount, email: 'hi@example.com' }}
      />,
    );
    const link = screen.getByRole('link', { name: 'hi@example.com' });
    expect(link).toHaveAttribute('href', 'mailto:hi@example.com');
  });
});

// ── Early adopter badge ───────────────────────────────────────────────────────

describe('ProfileViewSidebar early adopter badge', () => {
  it('shows the badge when account_number is within the first 1000', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} data={{ ...baseAccount, account_number: 42 }} />,
    );
    expect(screen.getByText('Early adopter')).toBeInTheDocument();
  });

  it('shows the account number on the badge', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} data={{ ...baseAccount, account_number: 42 }} />,
    );
    expect(screen.getByText('#42')).toBeInTheDocument();
  });

  it('hides the badge when account_number exceeds 1000', () => {
    renderWithProviders(
      <ProfileViewSidebar {...defaultProps} data={{ ...baseAccount, account_number: 1001 }} />,
    );
    expect(screen.queryByText('Early adopter')).not.toBeInTheDocument();
  });

  it('hides the badge when account_number is absent', () => {
    renderWithProviders(<ProfileViewSidebar {...defaultProps} />);
    expect(screen.queryByText('Early adopter')).not.toBeInTheDocument();
  });
});

// ── Organization links ────────────────────────────────────────────────────────

describe('ProfileViewSidebar organization links', () => {
  it('renders a link for each org', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...defaultProps}
        orgs={[
          { name: 'acme', display_name: 'Acme Corp' },
          { name: 'widget-co', display_name: 'Widget Co' },
        ]}
      />,
    );
    expect(screen.getByRole('link', { name: /acme corp/i })).toHaveAttribute('href', '/acme');
    expect(screen.getByRole('link', { name: /widget co/i })).toHaveAttribute('href', '/widget-co');
  });

  it('does not render the Organizations section when orgs is empty', () => {
    renderWithProviders(<ProfileViewSidebar {...defaultProps} orgs={[]} />);
    expect(screen.queryByText('Organizations')).not.toBeInTheDocument();
  });
});
