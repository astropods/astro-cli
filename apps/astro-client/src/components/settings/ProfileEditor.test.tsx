import { screen, cleanup, fireEvent, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, afterEach, vi } from 'vitest';
import type { ComponentProps } from 'react';
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

function renderEditor(
  readOnly: boolean,
  auth: AuthContextType = mockAuthContext,
  props?: Partial<ComponentProps<typeof ProfileEditor>>,
) {
  return renderRoute(
    [
      {
        path: '/',
        Component: () => (
          <ProfileEditor
            accountName={props?.accountName ?? "test-org"}
            currentDisplayName={props?.currentDisplayName ?? "Test Org"}
            avatarDialogTitle="Upload image"
            onSave={vi.fn().mockResolvedValue(undefined)}
            isSaving={false}
            readOnly={readOnly}
            {...props}
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
    const saveButton = screen.getByRole('button', { name: /^save$/i });
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

  it('keeps save actionable but blocks submitting when an organization display name exceeds its cap', () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderEditor(false, mockAuthContext, {
      displayNameKind: 'organization',
      onSave,
    });

    fireEvent.change(screen.getByDisplayValue('Test Org'), {
      target: { value: 'a'.repeat(40) },
    });

    expect(screen.getByText('Organization names cannot exceed 39 characters.')).toBeInTheDocument();
    const saveButton = screen.getByRole('button', { name: /^save$/i });
    expect(saveButton).toBeEnabled();
    fireEvent.click(saveButton);
    expect(onSave).not.toHaveBeenCalled();
  });

  it('keeps save actionable but blocks submitting when an organization display name is empty', () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderEditor(false, mockAuthContext, {
      displayNameKind: 'organization',
      onSave,
    });

    fireEvent.change(screen.getByDisplayValue('Test Org'), {
      target: { value: '   ' },
    });

    expect(screen.getByText("Organization name can't be empty")).toBeInTheDocument();
    const saveButton = screen.getByRole('button', { name: /^save$/i });
    expect(saveButton).toBeEnabled();
    fireEvent.click(saveButton);
    expect(onSave).not.toHaveBeenCalled();
  });

  it('keeps save actionable but blocks submitting when a personal display name is empty', () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderEditor(false, mockAuthContext, {
      displayNameKind: 'personal',
      onSave,
    });

    fireEvent.change(screen.getByDisplayValue('Test Org'), {
      target: { value: '   ' },
    });

    expect(screen.getByText("Display name can't be empty")).toBeInTheDocument();
    const saveButton = screen.getByRole('button', { name: /^save$/i });
    expect(saveButton).toBeEnabled();
    fireEvent.click(saveButton);
    expect(onSave).not.toHaveBeenCalled();
  });

  it('keeps the shared save spinner visible until onSave resolves', async () => {
    let resolveSave: () => void = () => {};
    const savePromise = new Promise<void>((resolve) => {
      resolveSave = resolve;
    });
    const onSave = vi.fn(() => savePromise);

    renderEditor(false, mockAuthContext, {
      onSave,
    });

    fireEvent.change(screen.getByDisplayValue('Test Org'), {
      target: { value: 'New Name' },
    });

    const saveButton = screen.getByRole('button', { name: /^save$/i });
    fireEvent.click(saveButton);

    expect(onSave).toHaveBeenCalledWith('New Name');
    expect(saveButton).toBeDisabled();
    expect(saveButton.querySelector('.animate-spin')).toBeInTheDocument();

    await act(async () => {
      resolveSave();
      await savePromise;
    });

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^save$/i })).not.toBeInTheDocument();
    });
  });

  it('shows an inline server error when saving fails', async () => {
    const onSave = vi.fn().mockRejectedValue({
      error_description: 'Server says no',
    });

    renderEditor(false, mockAuthContext, {
      onSave,
    });

    fireEvent.change(screen.getByDisplayValue('Test Org'), {
      target: { value: 'New Name' },
    });

    fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

    await waitFor(() => {
      expect(screen.getByText('Server says no')).toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
  });
});
