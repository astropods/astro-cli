import { screen, cleanup, fireEvent } from '@testing-library/react';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { ProfileEditor } from './ProfileEditor';

afterEach(cleanup);

function renderEditor(readOnly: boolean) {
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
      auth: mockAuthContext,
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
});
