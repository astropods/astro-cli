import { useCallback, useEffect, useRef, useState } from "react";

const SNAP_POINTS = [420, 600, 840];
const MIN_WIDTH = 320;

export interface SidePanelProps {
  open: boolean;
  /** Enable drag-to-resize handle on the left edge. Default: true. */
  resizable?: boolean;
  defaultWidth?: number;
  /** Called whenever the panel width changes (useful for parent layout decisions). */
  onWidthChange?: (width: number) => void;
  children: React.ReactNode;
}

export function SidePanel({
  open,
  resizable = true,
  defaultWidth = 420,
  onWidthChange,
  children,
}: SidePanelProps) {
  const [width, setWidth] = useState(defaultWidth);
  const widthRef = useRef(defaultWidth);
  const maxWidthRef = useRef(Math.floor(window.innerWidth * 0.5));
  const dragStartXRef = useRef<number | null>(null);
  const dragStartWidthRef = useRef<number>(defaultWidth);

  useEffect(() => {
    widthRef.current = width;
    onWidthChange?.(width);
  }, [width, onWidthChange]);

  useEffect(() => {
    const onResize = () => {
      maxWidthRef.current = Math.floor(window.innerWidth * 0.5);
    };
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  const handleDragMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    dragStartXRef.current = e.clientX;
    dragStartWidthRef.current = widthRef.current;

    const onMouseMove = (ev: MouseEvent) => {
      if (dragStartXRef.current === null) return;
      const delta = dragStartXRef.current - ev.clientX;
      const newWidth = Math.max(MIN_WIDTH, Math.min(maxWidthRef.current, dragStartWidthRef.current + delta));
      setWidth(newWidth);
    };

    const onMouseUp = (ev: MouseEvent) => {
      if (dragStartXRef.current === null) return;
      const delta = dragStartXRef.current - ev.clientX;
      const rawWidth = Math.max(MIN_WIDTH, Math.min(maxWidthRef.current, dragStartWidthRef.current + delta));
      const nearest = SNAP_POINTS.reduce((a, b) =>
        Math.abs(b - rawWidth) < Math.abs(a - rawWidth) ? b : a,
      );
      setWidth(nearest);
      dragStartXRef.current = null;
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };

    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
  }, []);

  return (
    <div
      style={{
        position: "relative",
        width: open ? width : 0,
        flexShrink: 0,
        overflowX: "clip",
        overflowY: "hidden",
        transition: "width 0.3s cubic-bezier(0.16, 1, 0.3, 1)",
        zIndex: 45,
      }}
    >
      {resizable && open && (
        <div
          onMouseDown={handleDragMouseDown}
          style={{
            position: "absolute",
            left: 0,
            top: 0,
            height: "100%",
            width: 8,
            zIndex: 50,
            cursor: "col-resize",
          }}
          className="hover:bg-primary/20"
        />
      )}
      <div className="flex h-full w-full flex-col border-l border-border bg-surface">
        {children}
      </div>
    </div>
  );
}
