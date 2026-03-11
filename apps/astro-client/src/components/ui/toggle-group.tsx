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
  items: string[];
}>({ value: undefined, items: [] });

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
  ...props
}: ToggleGroupSingleProps) {
  const [internalValue, setInternalValue] = React.useState(defaultValue);
  const activeValue = value ?? internalValue;

  const items = React.useMemo(() => {
    const values: string[] = [];
    React.Children.forEach(children, (child) => {
      if (React.isValidElement<{ value: string }>(child) && child.props.value) {
        values.push(child.props.value);
      }
    });
    return values;
  }, [children]);

  const activeIndex = activeValue ? items.indexOf(activeValue) : -1;

  // item size (24px) + gap (4px) = 32px total with p-1, matching default button h-8
  const itemSize = 24;
  const gap = 4;

  return (
    <ToggleGroupContext.Provider value={{ value: activeValue, items }}>
      <ToggleGroupPrimitive.Root
        className={cn(
          "relative inline-flex items-center gap-1 rounded-sm bg-secondary dark:bg-secondary p-1",
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
        {activeIndex >= 0 && (
          <div
            className="absolute rounded-sm bg-background dark:bg-background transition-transform duration-200 ease-in-out shadow-sm"
            style={{
              width: itemSize,
              height: itemSize,
              transform: `translateX(${activeIndex * (itemSize + gap)}px)`,
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
  const { value } = React.useContext(ToggleGroupContext);
  const isActive = value === props.value;

  const item = (
    <ToggleGroupPrimitive.Item
      className={cn(
        "relative z-10 inline-flex size-6 items-center justify-center rounded-[4px] text-muted-foreground transition-colors",
        "hover:text-primary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        "disabled:pointer-events-none disabled:opacity-50",
        isActive && "text-primary",
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
