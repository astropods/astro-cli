import { useState, useRef, useCallback } from "react";
import { Upload, X } from "lucide-react";
import { cn } from "@/lib/utils";

const DEFAULT_ACCEPT = ["image/jpeg", "image/png", "image/webp", "image/gif"];
const DEFAULT_MAX_SIZE = 5 << 20; // 5 MB

export interface ImageUploadProps {
  value: string | null;
  onSelect: (file: File, previewUrl: string) => void;
  onClear?: () => void;
  accept?: string[];
  maxSize?: number;
  hint?: string;
  className?: string;
  disabled?: boolean;
}

export function ImageUpload({
  value,
  onSelect,
  onClear,
  accept = DEFAULT_ACCEPT,
  maxSize = DEFAULT_MAX_SIZE,
  hint = "Drop an image here or click to browse",
  className,
  disabled = false,
}: ImageUploadProps) {
  const [isDragging, setIsDragging] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const prevUrlRef = useRef<string | null>(null);

  const validate = useCallback(
    (file: File): string | null => {
      if (!accept.includes(file.type)) {
        return `Unsupported format. Use ${accept.map((t) => t.split("/")[1]).join(", ")}.`;
      }
      if (file.size > maxSize) {
        return `File too large. Max ${Math.round(maxSize / (1 << 20))} MB.`;
      }
      return null;
    },
    [accept, maxSize],
  );

  const handleFile = useCallback(
    (file: File) => {
      setError(null);
      const err = validate(file);
      if (err) {
        setError(err);
        return;
      }
      // Revoke the previous URL if user selects a new file without clearing first
      if (prevUrlRef.current) {
        URL.revokeObjectURL(prevUrlRef.current);
      }
      const url = URL.createObjectURL(file);
      prevUrlRef.current = url;
      onSelect(file, url);
    },
    [validate, onSelect],
  );

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      if (!disabled) setIsDragging(true);
    },
    [disabled],
  );

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragging(false);
      if (disabled) return;
      const file = e.dataTransfer.files[0];
      if (file) handleFile(file);
    },
    [disabled, handleFile],
  );

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) handleFile(file);
      e.target.value = "";
    },
    [handleFile],
  );

  const handleClear = useCallback(() => {
    setError(null);
    if (prevUrlRef.current) {
      URL.revokeObjectURL(prevUrlRef.current);
      prevUrlRef.current = null;
    }
    onClear?.();
  }, [onClear]);

  if (value) {
    return (
      <div className={cn("flex flex-col items-center gap-3", className)}>
        <div className="relative">
          <img
            src={value}
            alt="Selected"
            className="size-32 rounded-full object-cover"
          />
          {onClear && !disabled && (
            <button
              type="button"
              onClick={handleClear}
              className="absolute -top-1 -right-1 flex size-6 items-center justify-center rounded-full bg-destructive text-destructive-foreground shadow-sm transition-opacity hover:opacity-90"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => !disabled && fileInputRef.current?.click()}
        className={cn(
          "flex flex-col items-center justify-center gap-2 rounded-[6px] border-2 border-dashed p-8 transition-colors",
          disabled
            ? "cursor-not-allowed opacity-50"
            : "cursor-pointer",
          isDragging
            ? "border-primary bg-primary/5"
            : "border-border hover:border-muted-foreground",
        )}
      >
        <Upload className="size-6 text-muted-foreground" />
        <p className="text-center text-sm text-muted-foreground">{hint}</p>
      </div>
      <input
        ref={fileInputRef}
        type="file"
        accept={accept.join(",")}
        onChange={handleInputChange}
        className="hidden"
      />
      {error && (
        <p className="text-xs text-destructive">{error}</p>
      )}
    </div>
  );
}
