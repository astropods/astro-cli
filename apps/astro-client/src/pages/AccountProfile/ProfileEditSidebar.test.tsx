import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/test-utils';
import { ProfileEditSidebar } from './ProfileEditSidebar';
import type { AccountPublic } from '@/lib/api';

afterEach(cleanup);

// ── Fixtures ──────────────────────────────────────────────────────────────────

const baseAccount: AccountPublic = {
  id: 'acct-1',
  name: 'testuser',
  type: 'personal',
  display_name: 'Test User',
  bio: 'Hello world',
  location: 'Earth',
  email: 'test@example.com',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

// ── Field pre-population ──────────────────────────────────────────────────────

describe('ProfileEditSidebar field pre-population', () => {
  it('pre-fills the display name field', () => {
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    expect(screen.getByDisplayValue('Test User')).toBeInTheDocument();
  });

  it('pre-fills the bio field', () => {
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    expect(screen.getByDisplayValue('Hello world')).toBeInTheDocument();
  });

  it('pre-fills the location field', () => {
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    expect(screen.getByDisplayValue('Earth')).toBeInTheDocument();
  });

  it('pre-fills the email field', () => {
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    expect(screen.getByDisplayValue('test@example.com')).toBeInTheDocument();
  });

  it('renders empty fields when optional props are absent', () => {
    renderWithProviders(
      <ProfileEditSidebar
        data={{ ...baseAccount, bio: undefined, location: undefined, email: undefined }}
        onClose={vi.fn()}
      />,
    );
    // Still renders the form — no crash
    expect(screen.getByRole('heading', { name: /edit profile/i })).toBeInTheDocument();
  });
});

// ── Display name validation ───────────────────────────────────────────────────

describe('ProfileEditSidebar display name validation', () => {
  it('Save button is enabled when display name is pre-filled', () => {
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
  });

  it('Save button is disabled and error appears when display name is cleared', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    await user.clear(screen.getByDisplayValue('Test User'));
    expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled();
    expect(screen.getByText("Display name can't be empty")).toBeInTheDocument();
  });

  it('Save button re-enables after typing a name back in', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    await user.clear(screen.getByDisplayValue('Test User'));
    await user.type(screen.getByPlaceholderText('Display name'), 'New Name');
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
    expect(screen.queryByText("Display name can't be empty")).not.toBeInTheDocument();
  });
});

// ── Close button ──────────────────────────────────────────────────────────────

describe('ProfileEditSidebar close button', () => {
  it('calls onClose when the × button is clicked', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={onClose} />,
    );
    // The × icon button is the only icon-sized button in the header
    await user.click(screen.getByRole('button', { name: '' }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('calls onClose when the Cancel button is clicked', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={onClose} />,
    );
    await user.click(screen.getByRole('button', { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
