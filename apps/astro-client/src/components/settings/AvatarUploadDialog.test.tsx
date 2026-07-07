import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cropImage } from '@/lib/crop-image';
import { AvatarUploadDialog } from './AvatarUploadDialog';

const cropArea = { x: 1, y: 2, width: 32, height: 32 };

vi.mock('@/components/ui/image-upload', () => ({
  ImageUpload: ({
    onSelect,
  }: {
    onSelect: (file: File, previewUrl: string) => void;
  }) => (
    <button
      type="button"
      onClick={() =>
        onSelect(
          new File(['raw image'], 'avatar.png', { type: 'image/png' }),
          'blob:preview',
        )
      }
    >
      Choose image
    </button>
  ),
}));

vi.mock('@/components/ui/image-cropper', () => ({
  ImageCropper: ({
    src,
    cropShape,
    onCropComplete,
  }: {
    src: string;
    cropShape?: 'rect' | 'round';
    onCropComplete: (area: typeof cropArea) => void;
  }) => (
    <div>
      <p>Cropper {src} {cropShape}</p>
      <button type="button" onClick={() => onCropComplete(cropArea)}>
        Set crop
      </button>
    </div>
  ),
}));

vi.mock('@/lib/crop-image', () => ({
  cropImage: vi.fn(),
}));

beforeEach(() => {
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('AvatarUploadDialog', () => {
  it('crops the selected image, uploads the blob, closes, and reports success', async () => {
    const user = userEvent.setup();
    const croppedBlob = new Blob(['cropped avatar'], { type: 'image/jpeg' });
    vi.mocked(cropImage).mockResolvedValue(croppedBlob);
    const onUpload = vi.fn().mockResolvedValue(undefined);
    const onSuccess = vi.fn();
    const onOpenChange = vi.fn();

    render(
      <AvatarUploadDialog
        open
        onOpenChange={onOpenChange}
        onUpload={onUpload}
        isPending={false}
        title="Upload profile image"
        onSuccess={onSuccess}
      />,
    );

    expect(screen.getByRole('button', { name: /^upload$/i })).toBeDisabled();

    await user.click(screen.getByRole('button', { name: /choose image/i }));
    expect(screen.getByText(/adjust the crop/i)).toBeInTheDocument();
    expect(screen.getByText('Cropper blob:preview round')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /set crop/i }));
    await user.click(screen.getByRole('button', { name: /^upload$/i }));

    await waitFor(() => expect(onUpload).toHaveBeenCalledWith(croppedBlob));
    expect(cropImage).toHaveBeenCalledWith('blob:preview', cropArea);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onSuccess).toHaveBeenCalledWith(croppedBlob);
  });

  it('keeps the dialog open and surfaces upload failures', async () => {
    const user = userEvent.setup();
    const croppedBlob = new Blob(['cropped avatar'], { type: 'image/jpeg' });
    vi.mocked(cropImage).mockResolvedValue(croppedBlob);
    const onUpload = vi.fn().mockRejectedValue({
      error_description: 'Avatar upload rejected',
    });
    const onSuccess = vi.fn();
    const onOpenChange = vi.fn();

    render(
      <AvatarUploadDialog
        open
        onOpenChange={onOpenChange}
        onUpload={onUpload}
        isPending={false}
        onSuccess={onSuccess}
      />,
    );

    await user.click(screen.getByRole('button', { name: /choose image/i }));
    await user.click(screen.getByRole('button', { name: /set crop/i }));
    await user.click(screen.getByRole('button', { name: /^upload$/i }));

    expect(await screen.findByText('Avatar upload rejected')).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(onSuccess).not.toHaveBeenCalled();
  });
});
