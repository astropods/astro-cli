import { screen, cleanup, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { bustAvatar } from '@/lib/avatar-bust';
import { ProfileEditor } from './ProfileEditor';
import type { AuthContextType } from '@/lib/auth-context';

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

function renderEditor(readOnly: boolean, auth: AuthContextType = mockAuthContext) {
  return renderRoute(
    [
      {
        path: '/',
        Component: () => (
          <ProfileEditor
            accountName="test-org"
            currentDisplayName="Test Org"
            avatarDialogTitle="Upload image"
            onSave={vi.fn().mockResolvedValue(undefined)}
            isSaving={false}
            readOnly={readOnly}
          />
        ),
      },
    ],
    {
      initialEntries: ['/'],
      auth,
    },
  );
}

describe('ProfileEditor', () => {
  it('readOnly disables display name input', () => {
    renderEditor(true);
    const input = screen.getByDisplayValue('Test Org');
    expect(input).toBeDisabled();
  });

  it('readOnly disables all buttons', () => {
    renderEditor(true);
    const buttons = screen.getAllByRole('button');
    buttons.forEach((btn) => {
      expect(btn).toBeDisabled();
    });
  });

  it('readOnly=false enables editing and save when dirty', () => {
    renderEditor(false);
    const input = screen.getByDisplayValue('Test Org');
    expect(input).not.toBeDisabled();
    // Make the form dirty so the save button becomes enabled
    fireEvent.change(input, { target: { value: 'New Name' } });
    const saveButton = screen.getByRole('button', { name: /save changes/i });
    expect(saveButton).not.toBeDisabled();
  });

  it('busts the avatar and refreshes auth data after upload success', async () => {
    const user = userEvent.setup();
    const refreshUserData = vi.fn().mockResolvedValue(undefined);
    const blob = new Blob(['avatar'], { type: 'image/png' });
    avatarUploadDialogMock.uploadBlob = blob;

    renderEditor(false, { ...mockAuthContext, refreshUserData });

    await user.click(screen.getByRole('button', { name: /test org/i }));
    await user.click(screen.getByRole('button', { name: /complete avatar upload/i }));

    expect(bustAvatar).toHaveBeenCalledWith('test-org', blob);
    expect(refreshUserData).toHaveBeenCalledOnce();
  });
});
