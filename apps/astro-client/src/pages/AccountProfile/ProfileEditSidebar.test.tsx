import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, cleanup, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { renderRoute, renderWithProviders, mockAuthContext } from '@/test/test-utils';
import { bustAvatar } from '@/lib/avatar-bust';
import { ProfileEditSidebar } from './ProfileEditSidebar';
import type { AccountPublic } from '@/lib/api';
import { ORG_DISPLAY_NAME_MAX_LENGTH } from '@/lib/constants';
import { server } from '@/test/msw/server';

const avatarUploadDialogMock = vi.hoisted(() => ({
  uploadBlob: undefined as Blob | undefined,
}));

vi.mock('@/lib/avatar-bust', async () => {
  const actual = await vi.importActual<typeof import('@/lib/avatar-bust')>('@/lib/avatar-bust');
  return {
    ...actual,
    bustAvatar: vi.fn(),
  };
});

vi.mock('@/components/settings/AvatarUploadDialog', () => ({
  AvatarUploadDialog: ({
    open,
    onSuccess,
  }: {
    open: boolean;
    onSuccess?: (blob: Blob) => void;
  }) =>
    open ? (
      <button
        type="button"
        onClick={() => {
          if (avatarUploadDialogMock.uploadBlob) {
            onSuccess?.(avatarUploadDialogMock.uploadBlob);
          }
        }}
      >
        Complete avatar upload
      </button>
    ) : null,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  avatarUploadDialogMock.uploadBlob = undefined;
});

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

const baseOrgAccount: AccountPublic = {
  ...baseAccount,
  id: 'org-1',
  name: 'test-org',
  type: 'organization',
  display_name: 'Test Org',
  email: undefined,
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

  it('Save button stays enabled and error appears when display name is cleared', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );
    await user.clear(screen.getByDisplayValue('Test User'));
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
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

  it('shows the shared organization name length error for org profiles', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ProfileEditSidebar data={baseOrgAccount} onClose={vi.fn()} variant="org" />,
    );

    await user.clear(screen.getByDisplayValue('Test Org'));
    await user.type(
      screen.getByPlaceholderText('Organization name'),
      'a'.repeat(ORG_DISPLAY_NAME_MAX_LENGTH + 1),
    );

    expect(
      screen.getByText(
        `Organization names cannot exceed ${ORG_DISPLAY_NAME_MAX_LENGTH} characters.`,
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
  });

  it('shows the shared empty organization name error for org profiles', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ProfileEditSidebar data={baseOrgAccount} onClose={vi.fn()} variant="org" />,
    );

    await user.clear(screen.getByDisplayValue('Test Org'));

    expect(screen.getByText("Organization name can't be empty")).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
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

// ── Avatar upload feedback ───────────────────────────────────────────────────

describe('ProfileEditSidebar avatar upload feedback', () => {
  it('busts the avatar and refreshes auth data after upload success', async () => {
    const user = userEvent.setup();
    const refreshUserData = vi.fn().mockResolvedValue(undefined);
    const blob = new Blob(['avatar'], { type: 'image/png' });
    avatarUploadDialogMock.uploadBlob = blob;

    renderRoute(
      [
        {
          path: '/',
          Component: () => (
            <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />
          ),
        },
      ],
      {
        initialEntries: ['/'],
        auth: { ...mockAuthContext, refreshUserData },
      },
    );

    await user.click(screen.getByRole('button', { name: /test user/i }));
    await user.click(screen.getByRole('button', { name: /complete avatar upload/i }));

    expect(bustAvatar).toHaveBeenCalledWith('testuser', blob);
    expect(refreshUserData).toHaveBeenCalledOnce();
  });
});

// ── Save consistency ─────────────────────────────────────────────────────────

describe('ProfileEditSidebar save consistency', () => {
  it('surfaces the server error when save fails', async () => {
    const user = userEvent.setup();

    server.use(
      http.patch('/api/v1/me', () =>
        HttpResponse.json(
          { error_description: 'Server says no' },
          { status: 400 },
        ),
      ),
      http.patch('/api/v1/accounts/testuser', () =>
        HttpResponse.json({ message: 'profile updated' }),
      ),
    );

    renderWithProviders(
      <ProfileEditSidebar data={baseAccount} onClose={vi.fn()} />,
    );

    await user.clear(screen.getByDisplayValue('Test User'));
    await user.type(screen.getByPlaceholderText('Display name'), 'New Name');
    await user.click(screen.getByRole('button', { name: /^save$/i }));

    await waitFor(() => {
      expect(screen.getByText('Server says no')).toBeInTheDocument();
    });
  });

  it('patches auth account data as soon as the org display-name save succeeds', async () => {
    const user = userEvent.setup();
    const patchAccount = vi.fn();
    const refreshUserData = vi.fn().mockResolvedValue(undefined);
    const onClose = vi.fn();
    let resolveProfileSave: () => void = () => {};
    const profileSave = new Promise<void>((resolve) => {
      resolveProfileSave = resolve;
    });

    server.use(
      http.patch('/api/v1/accounts/test-org', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>;
        if ('display_name' in body) {
          return HttpResponse.json({ message: 'profile updated' });
        }

        await profileSave;
        return HttpResponse.json({ message: 'profile updated' });
      }),
    );

    renderRoute(
      [
        {
          path: '/',
          Component: () => (
            <ProfileEditSidebar
              data={baseOrgAccount}
              onClose={onClose}
              variant="org"
            />
          ),
        },
      ],
      {
        initialEntries: ['/'],
        auth: {
          ...mockAuthContext,
          accounts: [baseOrgAccount],
          patchAccount,
          refreshUserData,
        },
      },
    );

    await user.clear(screen.getByDisplayValue('Test Org'));
    await user.type(screen.getByPlaceholderText('Organization name'), 'New Org');
    await user.click(screen.getByRole('button', { name: /^save$/i }));

    await waitFor(() => {
      expect(patchAccount).toHaveBeenCalledWith('test-org', { display_name: 'New Org' });
    });
    expect(refreshUserData).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();

    resolveProfileSave();

    await waitFor(() => {
      expect(refreshUserData).toHaveBeenCalledOnce();
    });
    expect(onClose).toHaveBeenCalledOnce();
  });
});
