import * as React from "react";
import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

const ToggleGroupContext = React.createContext<{
  value: string | undefined;
  variant: "icon" | "word";
}>({ value: undefined, variant: "icon" });

type ToggleGroupSingleProps = Extract<
  React.ComponentProps<typeof ToggleGroupPrimitive.Root>,
  { type: "single" }
>;

function ToggleGroup({
  className,
  children,
  value,
  defaultValue,
  onValueChange,
  type,
  variant = "icon",
  ...props
}: ToggleGroupSingleProps & { variant?: "icon" | "word" }) {
  const [internalValue, setInternalValue] = React.useState(defaultValue);
  const activeValue = value ?? internalValue;
  const rootRef = React.useRef<HTMLDivElement>(null);
  const [indicator, setIndicator] = React.useState({ left: 0, width: 0, ready: false });

  React.useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const activeEl = root.querySelector<HTMLElement>('[data-state="on"]');
    if (!activeEl) {
      setIndicator((prev) => ({ ...prev, ready: false }));
      return;
    }
    setIndicator({ left: activeEl.offsetLeft, width: activeEl.offsetWidth, ready: true });
  }, [activeValue, children, variant]);

  return (
    <ToggleGroupContext.Provider value={{ value: activeValue, variant }}>
      <ToggleGroupPrimitive.Root
        ref={rootRef}
        className={cn(
          "relative inline-flex items-center gap-1",
          variant === "icon"
            ? "rounded-sm bg-secondary dark:bg-secondary p-1"
            : "rounded-[7px] border border-border bg-muted p-[2px] gap-0.5",
          className
        )}
        type={type}
        value={value}
        defaultValue={defaultValue}
        onValueChange={(v) => {
          if (!v) return;
          setInternalValue(v);
          onValueChange?.(v);
        }}
        {...props}
      >
        {indicator.ready && (
          <div
            className={cn(
              "absolute transition-all duration-200 ease-out",
              variant === "icon"
                ? "rounded-sm bg-background dark:bg-background shadow-sm"
                : "rounded-[6px] bg-surface border border-border/70"
            )}
            style={{
              top: variant === "icon" ? 4 : 2,
              left: indicator.left,
              width: indicator.width,
              height: variant === "icon" ? 24 : "calc(100% - 4px)",
            }}
          />
        )}
        {children}
      </ToggleGroupPrimitive.Root>
    </ToggleGroupContext.Provider>
  );
}

function ToggleGroupItem({
  className,
  children,
  tooltip,
  ...props
}: React.ComponentProps<typeof ToggleGroupPrimitive.Item> & {
  tooltip?: string;
}) {
  const { value, variant } = React.useContext(ToggleGroupContext);
  const isActive = value === props.value;

  const item = (
    <ToggleGroupPrimitive.Item
      className={cn(
        "relative z-10 inline-flex items-center justify-center transition-colors",
        variant === "icon"
          ? "size-6 rounded-[4px] text-muted-foreground hover:text-primary"
          : "rounded-[6px] px-3.5 py-1.5 text-body leading-none",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        "disabled:pointer-events-none disabled:opacity-50",
        variant === "icon"
          ? isActive && "text-primary"
          : isActive
            ? "text-foreground font-medium"
            : "text-faint-foreground hover:text-foreground",
        className
      )}
      {...props}
    >
      {children}
    </ToggleGroupPrimitive.Item>
  );

  if (!tooltip) return item;

  return (
    <TooltipProvider delayDuration={500}>
      <Tooltip>
        <TooltipTrigger asChild>{item}</TooltipTrigger>
        <TooltipContent sideOffset={8}>{tooltip}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export { ToggleGroup, ToggleGroupItem };
