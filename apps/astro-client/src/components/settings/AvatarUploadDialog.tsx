import { useState, useCallback, useRef, useEffect } from "react";
import { Loader2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { ImageUpload } from "@/components/ui/image-upload";
import { ImageCropper } from "@/components/ui/image-cropper";
import { cropImage, type CropArea } from "@/lib/crop-image";
import { useUploadAvatar } from "@/api/queries";

interface AvatarUploadDialogProps {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
}

export function AvatarUploadDialog({
  account,
  open,
  onOpenChange,
  onSuccess,
}: AvatarUploadDialogProps) {
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const cropAreaRef = useRef<CropArea | null>(null);
  const previewUrlRef = useRef<string | null>(null);
  previewUrlRef.current = previewUrl;
  const uploadAvatar = useUploadAvatar();

  useEffect(() => {
    return () => {
      if (previewUrlRef.current) URL.revokeObjectURL(previewUrlRef.current);
    };
  }, []);

  const reset = useCallback(() => {
    setPreviewUrl((prev) => {
      if (prev) URL.revokeObjectURL(prev);
      return null;
    });
    setError(null);
    cropAreaRef.current = null;
    uploadAvatar.reset();
  }, [uploadAvatar]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) reset();
      onOpenChange(next);
    },
    [onOpenChange, reset],
  );

  const handleSelect = useCallback((_file: File, url: string) => {
    setPreviewUrl(url);
    setError(null);
  }, []);

  const handleCropComplete = useCallback((area: CropArea) => {
    cropAreaRef.current = area;
  }, []);

  const handleUpload = useCallback(async () => {
    if (!previewUrl || !cropAreaRef.current) return;

    setError(null);
    try {
      const blob = await cropImage(previewUrl, cropAreaRef.current);
      await uploadAvatar.mutateAsync({ account, file: blob });
      handleOpenChange(false);
      onSuccess?.();
    } catch (err) {
      setError(
        err && typeof err === "object" && "error_description" in err
          ? String((err as { error_description: string }).error_description)
          : "Upload failed. Please try again.",
      );
    }
  }, [previewUrl, account, uploadAvatar, handleOpenChange, onSuccess]);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex h-[85dvh] w-[calc(100vw-2rem)] max-w-lg flex-col overflow-y-auto sm:h-auto">
        <DialogHeader>
          <DialogTitle>Upload profile image</DialogTitle>
          <DialogDescription>
            {previewUrl
              ? "Adjust the crop, then upload."
              : "Choose an image for your profile."}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1">
          {previewUrl ? (
            <ImageCropper
              src={previewUrl}
              onCropComplete={handleCropComplete}
              className="h-full"
            />
          ) : (
            <ImageUpload
              value={null}
              onSelect={handleSelect}
              className="h-full [&>div:first-child]:flex-1"
            />
          )}
        </div>

        {error && (
          <p className="text-xs text-destructive">{error}</p>
        )}

        <DialogFooter>
          {previewUrl && (
            <Button
              variant="outline"
              onClick={reset}
              disabled={uploadAvatar.isPending}
            >
              Back
            </Button>
          )}
          <Button
            onClick={handleUpload}
            disabled={!previewUrl || uploadAvatar.isPending}
          >
            {uploadAvatar.isPending && (
              <Loader2 size={14} className="spinner-delayed" />
            )}
            Upload
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
