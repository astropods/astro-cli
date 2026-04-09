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

export interface AvatarUploadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUpload: (file: Blob) => Promise<void>;
  isPending: boolean;
  title?: string;
  description?: string;
  cropShape?: "rect" | "round";
  onSuccess?: (blob: Blob) => void;
}

export function AvatarUploadDialog({
  open,
  onOpenChange,
  onUpload,
  isPending,
  title = "Upload image",
  description,
  cropShape = "round",
  onSuccess,
}: AvatarUploadDialogProps) {
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const cropAreaRef = useRef<CropArea | null>(null);
  const previewUrlRef = useRef<string | null>(null);
  previewUrlRef.current = previewUrl;

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
  }, []);

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
      await onUpload(blob);
      handleOpenChange(false);
      onSuccess?.(blob);
    } catch (err) {
      setError(
        err && typeof err === "object" && "error_description" in err
          ? String((err as { error_description: string }).error_description)
          : "Upload failed. Please try again.",
      );
    }
  }, [previewUrl, onUpload, handleOpenChange, onSuccess]);

  const defaultDescription = previewUrl
    ? "Adjust the crop, then upload."
    : "Choose an image.";

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex h-[85dvh] w-[calc(100vw-2rem)] max-w-lg flex-col overflow-y-auto sm:h-auto">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {description ?? defaultDescription}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1">
          {previewUrl ? (
            <ImageCropper
              src={previewUrl}
              cropShape={cropShape}
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
              disabled={isPending}
            >
              Back
            </Button>
          )}
          <Button
            onClick={handleUpload}
            disabled={!previewUrl || isPending}
          >
            {isPending && (
              <Loader2 size={14} className="spinner-delayed" />
            )}
            Upload
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
