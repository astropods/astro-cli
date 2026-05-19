import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { ProfileViewSidebar } from './ProfileViewSidebar';
import type { AccountPublic, AccountMember } from '@/lib/api';

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

const baseOrg: AccountPublic = {
  id: 'acct-org',
  name: 'astropods',
  type: 'organization',
  display_name: 'Astro Pods',
  created_at: '2025-01-15T00:00:00Z',
  updated_at: '2025-01-15T00:00:00Z',
};

function makeMember(overrides?: Partial<AccountMember>): AccountMember {
  return {
    account_id: 'acct-1',
    user_id: 'user-1',
    role: 'admin',
    status: 'active',
    username: 'testuser',
    display_name: 'Test User',
    created_at: '2025-01-01T00:00:00Z',
    slack_workspaces: [],
    ...overrides,
  };
}

const personalDefaults = {
  data: baseAccount,
  isAdmin: false,
  blueprintCount: 0,
  deploymentCount: 0,
  orgs: [],
};

const orgDefaults = {
  data: baseOrg,
  variant: 'org' as const,
  isAdmin: false,
  blueprintCount: 0,
  deploymentCount: 0,
  members: [],
};

// ── Personal: display name ────────────────────────────────────────────────────

describe('ProfileViewSidebar display name', () => {
  it('renders display_name when set', () => {
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} />);
    expect(screen.getByRole('heading', { name: 'Test User' })).toBeInTheDocument();
  });

  it('falls back to account name when display_name is absent', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, display_name: undefined }} />,
    );
    expect(screen.getByRole('heading', { name: 'testuser' })).toBeInTheDocument();
  });

  it('renders the @handle below the display name', () => {
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} />);
    expect(screen.getByText('@testuser')).toBeInTheDocument();
  });
});

// ── Personal: edit profile button ─────────────────────────────────────────────

describe('ProfileViewSidebar edit profile button', () => {
  it('shows the edit button when isAdmin and onEditOpen are both provided', () => {
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} isAdmin onEditOpen={vi.fn()} />);
    expect(screen.getByRole('button', { name: /edit profile/i })).toBeInTheDocument();
  });

  it('hides the edit button when isAdmin is false', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} isAdmin={false} onEditOpen={vi.fn()} />,
    );
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });

  it('hides the edit button when onEditOpen is undefined (external view)', () => {
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} isAdmin onEditOpen={undefined} />);
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });

  it('calls onEditOpen when the edit button is clicked', async () => {
    const user = userEvent.setup();
    const onEditOpen = vi.fn();
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} isAdmin onEditOpen={onEditOpen} />);
    await user.click(screen.getByRole('button', { name: /edit profile/i }));
    expect(onEditOpen).toHaveBeenCalledOnce();
  });
});

// ── Personal: stats ───────────────────────────────────────────────────────────

describe('ProfileViewSidebar stats', () => {
  it('shows the Blueprints stat', () => {
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} blueprintCount={5} />);
    expect(screen.getByText('Blueprints')).toBeInTheDocument();
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('shows the Agents stat for the owner', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} isAdmin isInternalView deploymentCount={3} />,
    );
    expect(screen.getByText('Agents')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('hides the Agents stat for a visitor', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} isAdmin={false} deploymentCount={3} />,
    );
    expect(screen.queryByText('Agents')).not.toBeInTheDocument();
  });
});

// ── Personal: optional meta fields ───────────────────────────────────────────

describe('ProfileViewSidebar optional meta fields', () => {
  it('renders bio when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, bio: 'I build things.' }} />,
    );
    expect(screen.getByText('I build things.')).toBeInTheDocument();
  });

  it('renders pronouns when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, pronouns: 'they/them' }} />,
    );
    expect(screen.getByText('they/them')).toBeInTheDocument();
  });

  it('renders location when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, location: 'San Francisco' }} />,
    );
    expect(screen.getByText('San Francisco')).toBeInTheDocument();
  });

  it('renders email as a mailto link', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, email: 'hi@example.com' }} />,
    );
    const link = screen.getByRole('link', { name: 'hi@example.com' });
    expect(link).toHaveAttribute('href', 'mailto:hi@example.com');
  });
});

// ── Personal: early adopter badge ─────────────────────────────────────────────

describe('ProfileViewSidebar early adopter badge', () => {
  it('shows the badge when account_number is within the first 1000', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, account_number: 42 }} />,
    );
    expect(screen.getByText('Early adopter')).toBeInTheDocument();
  });

  it('shows the account number on the badge', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, account_number: 42 }} />,
    );
    expect(screen.getByText('#42')).toBeInTheDocument();
  });

  it('hides the badge when account_number exceeds 1000', () => {
    renderWithProviders(
      <ProfileViewSidebar {...personalDefaults} data={{ ...baseAccount, account_number: 1001 }} />,
    );
    expect(screen.queryByText('Early adopter')).not.toBeInTheDocument();
  });

  it('hides the badge when account_number is absent', () => {
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} />);
    expect(screen.queryByText('Early adopter')).not.toBeInTheDocument();
  });
});

// ── Personal: organization links ──────────────────────────────────────────────

describe('ProfileViewSidebar organization links', () => {
  it('renders a link for each org', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...personalDefaults}
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
    renderWithProviders(<ProfileViewSidebar {...personalDefaults} orgs={[]} />);
    expect(screen.queryByText('Organizations')).not.toBeInTheDocument();
  });
});

// ── Org: identity ─────────────────────────────────────────────────────────────

describe('ProfileViewSidebar org identity', () => {
  it('renders the display name', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} />);
    expect(screen.getByRole('heading', { name: 'Astro Pods' })).toBeInTheDocument();
  });

  it('falls back to account name when display_name is absent', () => {
    renderWithProviders(
      <ProfileViewSidebar {...orgDefaults} data={{ ...baseOrg, display_name: undefined }} />,
    );
    expect(screen.getByRole('heading', { name: 'astropods' })).toBeInTheDocument();
  });

  it('renders the @handle', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} />);
    expect(screen.getByText('@astropods')).toBeInTheDocument();
  });

  it('shows "Founded" (not "Joined") for the creation date', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} />);
    expect(screen.getByText(/founded/i)).toBeInTheDocument();
    expect(screen.queryByText(/joined/i)).not.toBeInTheDocument();
  });
});

// ── Org: optional meta fields ─────────────────────────────────────────────────

describe('ProfileViewSidebar org optional meta fields', () => {
  it('renders bio when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar {...orgDefaults} data={{ ...baseOrg, bio: 'Building the future.' }} />,
    );
    expect(screen.getByText('Building the future.')).toBeInTheDocument();
  });

  it('renders location when provided', () => {
    renderWithProviders(
      <ProfileViewSidebar {...orgDefaults} data={{ ...baseOrg, location: 'San Francisco, CA' }} />,
    );
    expect(screen.getByText('San Francisco, CA')).toBeInTheDocument();
  });

  it('renders website as an external link', () => {
    renderWithProviders(
      <ProfileViewSidebar {...orgDefaults} data={{ ...baseOrg, website: 'https://astropods.ai' }} />,
    );
    const link = screen.getByRole('link', { name: 'astropods.ai' });
    expect(link).toHaveAttribute('href', 'https://astropods.ai');
  });

  it('does not render email or pronouns fields', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        data={{ ...baseOrg, email: 'hello@astropods.ai', pronouns: 'they/them' } as AccountPublic}
      />,
    );
    expect(screen.queryByText('hello@astropods.ai')).not.toBeInTheDocument();
    expect(screen.queryByText('they/them')).not.toBeInTheDocument();
  });
});

// ── Org: edit profile button ──────────────────────────────────────────────────

describe('ProfileViewSidebar org edit profile button', () => {
  it('shows the button when isAdmin and onEditOpen are both provided', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isAdmin onEditOpen={vi.fn()} />);
    expect(screen.getByRole('button', { name: /edit profile/i })).toBeInTheDocument();
  });

  it('hides the button when isAdmin is false', () => {
    renderWithProviders(
      <ProfileViewSidebar {...orgDefaults} isAdmin={false} onEditOpen={vi.fn()} />,
    );
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });

  it('hides the button when onEditOpen is undefined (visitor view)', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isAdmin onEditOpen={undefined} />);
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });

  it('calls onEditOpen when clicked', async () => {
    const user = userEvent.setup();
    const onEditOpen = vi.fn();
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isAdmin onEditOpen={onEditOpen} />);
    await user.click(screen.getByRole('button', { name: /edit profile/i }));
    expect(onEditOpen).toHaveBeenCalledOnce();
  });
});

// ── Org: stats ────────────────────────────────────────────────────────────────

describe('ProfileViewSidebar org stats', () => {
  it('shows blueprints count for visitors', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isAdmin={false} blueprintCount={7} />);
    expect(screen.getByText('Blueprints')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  it('shows agents count for any org member', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isInternalView deploymentCount={4} />);
    expect(screen.getByText('Agents')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
  });

  it('hides agents count for visitors', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isAdmin={false} deploymentCount={4} />);
    expect(screen.queryByText('Agents')).not.toBeInTheDocument();
  });
});

// ── Org: members section ──────────────────────────────────────────────────────

describe('ProfileViewSidebar org members', () => {
  // Members section is gated by isInternalView (mirrors the server-side membership
  // gate). Each rendering case sets it to true; the visitor case is exercised by
  // AccountProfile.test.tsx end-to-end.
  it('renders active members', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        isInternalView
        members={[makeMember({ username: 'testuser', status: 'active' })]}
      />,
    );
    expect(screen.getByText(/members/i)).toBeInTheDocument();
  });

  it('does not render the members section when there are no active members', () => {
    renderWithProviders(<ProfileViewSidebar {...orgDefaults} isInternalView members={[]} />);
    expect(screen.queryByText(/^members$/i)).not.toBeInTheDocument();
  });

  it('does not render the members section when not an internal view', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        members={[makeMember({ username: 'testuser', status: 'active' })]}
      />,
    );
    expect(screen.queryByText(/^members$/i)).not.toBeInTheDocument();
  });

  it('filters out pending members', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        isInternalView
        members={[
          makeMember({ username: 'active-user', display_name: 'Active User', status: 'active' }),
          makeMember({ username: 'pending-user', display_name: 'Pending User', user_id: 'user-2', status: 'pending' }),
        ]}
      />,
    );
    expect(screen.getByRole('link', { name: 'Active User' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Pending User' })).not.toBeInTheDocument();
  });

  it('shows multiple members', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        isInternalView
        members={[
          makeMember({ username: 'alice', display_name: 'Alice', user_id: 'u1', status: 'active' }),
          makeMember({ username: 'bob', display_name: 'Bob', user_id: 'u2', status: 'active' }),
        ]}
      />,
    );
    expect(screen.getByRole('link', { name: 'Alice' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Bob' })).toBeInTheDocument();
  });

  it('filters out active members who have no username', () => {
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        isInternalView
        members={[
          makeMember({ username: 'alice', display_name: 'Alice', user_id: 'u1', status: 'active' }),
          makeMember({ username: '', display_name: 'No Handle', user_id: 'u2', status: 'active' }),
        ]}
      />,
    );
    expect(screen.getByRole('link', { name: 'Alice' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'No Handle' })).not.toBeInTheDocument();
  });

  it('excludes handleless members from the View all count', () => {
    // 13 members with usernames + 1 without → View all should show 13, not 14
    const namedMembers = Array.from({ length: 13 }, (_, i) =>
      makeMember({ username: `user-${i}`, display_name: `User ${i}`, user_id: `u${i}`, status: 'active' }),
    );
    renderWithProviders(
      <ProfileViewSidebar
        {...orgDefaults}
        isInternalView
        members={[
          ...namedMembers,
          makeMember({ username: '', display_name: 'No Handle', user_id: 'nohandle', status: 'active' }),
        ]}
      />,
    );
    expect(screen.getByRole('button', { name: /view all 13/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /view all 14/i })).not.toBeInTheDocument();
  });
});
