import { useState, useCallback, useRef, useEffect } from "react";
import Cropper from "react-easy-crop";
import type { Area, MediaSize } from "react-easy-crop";
import type { CropArea } from "@/lib/crop-image";
import { cn } from "@/lib/utils";

export interface ImageCropperProps {
  src: string;
  aspect?: number;
  cropShape?: "rect" | "round";
  onCropComplete: (area: CropArea) => void;
  className?: string;
}

export function ImageCropper({
  src,
  aspect = 1,
  cropShape = "round",
  onCropComplete,
  className,
}: ImageCropperProps) {
  const [crop, setCrop] = useState({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [minZoom, setMinZoom] = useState(1);
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerSize, setContainerSize] = useState<number | null>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      setContainerSize(entry.contentRect.width);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const handleMediaLoaded = useCallback(
    (mediaSize: MediaSize) => {
      if (!containerSize) return;
      // At zoom=1 with objectFit="contain", react-easy-crop scales the image
      // so it fits inside the container. mediaSize.width/height are those
      // displayed dimensions. We need enough zoom so the shorter displayed
      // dimension covers the square crop.
      const coverZoom = Math.max(
        containerSize / mediaSize.width,
        containerSize / mediaSize.height,
      );
      setMinZoom(coverZoom);
      setZoom(coverZoom);
    },
    [containerSize],
  );

  const handleCropComplete = useCallback(
    (_croppedArea: Area, croppedAreaPixels: Area) => {
      onCropComplete(croppedAreaPixels);
    },
    [onCropComplete],
  );

  return (
    <div className={cn("flex min-h-0 flex-col gap-3", className)}>
      <div ref={containerRef} className="relative min-h-0 flex-1 aspect-square w-full overflow-hidden rounded-md bg-white">
        {containerSize != null && (
          <Cropper
            image={src}
            crop={crop}
            zoom={zoom}
            minZoom={minZoom}
            maxZoom={minZoom * 3}
            aspect={aspect}
            cropShape={cropShape}
            cropSize={{ width: containerSize, height: containerSize }}
            mediaProps={{ style: { background: "white" } }}
            style={{ containerStyle: { background: "white" } }}
            onMediaLoaded={handleMediaLoaded}
            onCropChange={setCrop}
            onZoomChange={setZoom}
            onCropComplete={handleCropComplete}
          />
        )}
      </div>
      <div className="flex items-center gap-3 px-1">
        <span className="text-xs text-muted-foreground">Zoom</span>
        <input
          type="range"
          min={minZoom}
          max={minZoom * 3}
          step={0.01}
          value={zoom}
          onChange={(e) => setZoom(Number(e.target.value))}
          className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-border accent-primary"
        />
      </div>
    </div>
  );
}
