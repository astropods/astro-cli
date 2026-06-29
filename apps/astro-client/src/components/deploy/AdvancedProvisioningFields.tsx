import { useState } from "react";
import { ChevronRight, Lock, RotateCcw } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import { DEFAULT_AGENT_VOLUME_MOUNT } from "./constants";

// Placeholder unit prices — replace with real billing data when the API is
// available. Modeled loosely on managed-container pricing.
const PRICE_PER_VCPU_HOUR = 0.04;        // USD per vCPU per hour
const PRICE_PER_GI_RAM_HOUR = 0.005;     // USD per GiB RAM per hour
const PRICE_PER_GI_STORAGE_MONTH = 0.10; // USD per GiB-month for PVCs
const HOURS_PER_MONTH = 730;

/**
 * Parse a K8s CPU quantity ("250m", "0.5", "2") into a vCPU number.
 * Defaults to 0 on bad input.
 */
function parseCpu(value: string): number {
  if (!value) return 0;
  if (value.endsWith("m")) {
    const n = parseFloat(value.slice(0, -1));
    return Number.isFinite(n) ? n / 1000 : 0;
  }
  const n = parseFloat(value);
  return Number.isFinite(n) ? n : 0;
}

/**
 * Parse a K8s memory/storage quantity ("512Mi", "1Gi") into GiB.
 * Supports Mi and Gi suffixes; defaults to 0 on bad input.
 */
function parseMemoryGi(value: string): number {
  if (!value) return 0;
  if (value.endsWith("Gi")) {
    const n = parseFloat(value.slice(0, -2));
    return Number.isFinite(n) ? n : 0;
  }
  if (value.endsWith("Mi")) {
    const n = parseFloat(value.slice(0, -2));
    return Number.isFinite(n) ? n / 1024 : 0;
  }
  return 0;
}

function formatUSD(amount: number): string {
  if (amount < 1) return `$${amount.toFixed(2)}`;
  return amount.toLocaleString("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: amount < 10 ? 2 : 0,
  });
}

export interface AdvancedProvisioningFieldsProps {
  cpu: string;
  memory: string;
  /** Whether persistent volume is enabled — gates the storage slider. */
  /** Mount path for the persistent disk. Empty falls back to the default. */
  mountPath: string;
  storageSize: string;
  /**
   * When true, the storage slider is locked. Storage on a live PVC cannot be
   * resized in place (the StatefulSet's volumeClaimTemplates is immutable),
   * so we lock the control once the deployment exists.
   */
  storageLocked?: boolean;
  onCpuChange: (value: string) => void;
  onMemoryChange: (value: string) => void;
  onMountPathChange: (value: string) => void;
  onStorageSizeChange: (value: string) => void;
}


const CPU_TIERS = ["25m", "50m", "100m", "250m", "500m", "1"];

const MEMORY_TIERS = ["256Mi", "512Mi", "1Gi", "2Gi", "4Gi"];

const STORAGE_TIERS = ["5Gi", "10Gi", "20Gi", "30Gi", "50Gi"];

const DEFAULT_CPU_INDEX = 2;     // "100m"
const DEFAULT_MEMORY_INDEX = 2;  // "1Gi"
const DEFAULT_STORAGE_INDEX = 0; // "5Gi" — matches the server-side default disk

function indexOf(value: string, tiers: string[], fallback: number): number {
  const i = tiers.indexOf(value);
  return i === -1 ? fallback : i;
}

/**
 * One unified panel for the agent's compute and storage knobs. The whole
 * surface is a single Card; the header doubles as the collapsible trigger
 * and stays visible whether the section is open or closed. Sliders snap to
 * predefined tiers; clearing a field lets the server fall back to
 * astropods.yml + tier defaults.
 */
export function AdvancedProvisioningFields({
  cpu,
  memory,
  mountPath,
  storageSize,
  storageLocked,
  onCpuChange,
  onMemoryChange,
  onMountPathChange,
  onStorageSizeChange,
}: AdvancedProvisioningFieldsProps) {
  const [open, setOpen] = useState(false);

  const cpuIndex = cpu ? indexOf(cpu, CPU_TIERS, DEFAULT_CPU_INDEX) : DEFAULT_CPU_INDEX;
  const memoryIndex = memory ? indexOf(memory, MEMORY_TIERS, DEFAULT_MEMORY_INDEX) : DEFAULT_MEMORY_INDEX;
  const storageIndex = storageSize ? indexOf(storageSize, STORAGE_TIERS, DEFAULT_STORAGE_INDEX) : DEFAULT_STORAGE_INDEX;

  // Live cost estimate based on the currently displayed tier values.
  const effectiveCpu = parseCpu(cpu || CPU_TIERS[cpuIndex]);
  const effectiveMemoryGi = parseMemoryGi(memory || MEMORY_TIERS[memoryIndex]);
  const effectiveStorageGi = parseMemoryGi(storageSize || STORAGE_TIERS[storageIndex]);
  const cpuMonthly = effectiveCpu * PRICE_PER_VCPU_HOUR * HOURS_PER_MONTH;
  const ramMonthly = effectiveMemoryGi * PRICE_PER_GI_RAM_HOUR * HOURS_PER_MONTH;
  const storageMonthly = effectiveStorageGi * PRICE_PER_GI_STORAGE_MONTH;
  const totalMonthly = cpuMonthly + ramMonthly + storageMonthly;

  return (
    <div className="overflow-hidden rounded-[10px] border border-border">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger
          className={cn(
            "flex w-full items-center gap-3 bg-muted px-5 py-3 text-left",
            "transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          )}
        >
          <ChevronRight
            className={cn(
              "size-4 text-muted-foreground transition-transform",
              open && "rotate-90",
            )}
          />
          <span className="text-heading-4 text-foreground">Advanced sizing</span>
        </CollapsibleTrigger>

        <CollapsibleContent>
          <div className="space-y-6 border-t border-border bg-muted px-5 py-5">
            <TierSlider
              label="CPU"
              description="Cores allocated to the agent container."
              tiers={CPU_TIERS}
              displayLabels={CPU_TIERS}
              value={cpu}
              index={cpuIndex}
              isOverridden={!!cpu}
              onChange={onCpuChange}
            />

            <TierSlider
              label="Memory"
              description="RAM allocated to the agent container."
              tiers={MEMORY_TIERS}
              displayLabels={MEMORY_TIERS}
              value={memory}
              index={memoryIndex}
              isOverridden={!!memory}
              onChange={onMemoryChange}
            />

            <div className="space-y-5 border-t border-border pt-5">
              <div className="space-y-1.5">
                <Label size="md" htmlFor="agent-volume-mount">Mount path</Label>
                <Input
                  id="agent-volume-mount"
                  value={mountPath}
                  onChange={(e) => onMountPathChange(e.target.value)}
                  placeholder={DEFAULT_AGENT_VOLUME_MOUNT}
                />
                <p className="text-body-sm text-muted-foreground">
                  Absolute path where the persistent disk is mounted in the agent
                  container. Defaults to {DEFAULT_AGENT_VOLUME_MOUNT}.
                </p>
              </div>
              <TierSlider
                label="Storage"
                description="Persistent disk for the agent. Set the size now — it's fixed once the agent is deployed."
                tiers={STORAGE_TIERS}
                displayLabels={STORAGE_TIERS}
                value={storageSize}
                index={storageIndex}
                isOverridden={!!storageSize}
                disabled={storageLocked}
                disabledHint={
                  storageLocked
                    ? "Disk size is locked after first deploy."
                    : undefined
                }
                onChange={onStorageSizeChange}
              />
            </div>
          </div>
        </CollapsibleContent>

        <div className="flex flex-col gap-3 border-t border-border px-5 py-3 sm:flex-row sm:items-center sm:gap-6">
          <div className="flex flex-wrap gap-x-6 gap-y-2">
            <CostItem label="CPU" value={cpuMonthly} />
            <CostItem label="Memory" value={ramMonthly} />
            {storageMonthly > 0 && <CostItem label="Storage" value={storageMonthly} />}
          </div>
          <div className="flex items-center gap-4 sm:ml-auto">
            <div className="hidden h-9 w-px bg-border sm:block" />
            <div className="sm:text-right">
              <div className="text-mono-sm text-faint-foreground">Estimated total</div>
              <div className="mt-1 text-heading-3 tabular-nums text-foreground">
                ~{formatUSD(totalMonthly)}
                <span className="text-body-sm font-normal text-muted-foreground"> /mo</span>
              </div>
            </div>
          </div>
        </div>
      </Collapsible>
    </div>
  );
}

interface TierSliderProps {
  label: string;
  description: string;
  tiers: string[];
  displayLabels: string[];
  displayUnit?: string;
  value: string;
  index: number;
  isOverridden: boolean;
  /** Lock the slider — used after first deploy when the value can no longer change. */
  disabled?: boolean;
  /** Helper text shown under the description when disabled. */
  disabledHint?: string;
  onChange: (value: string) => void;
}

function TierSlider({
  label,
  description,
  tiers,
  displayLabels,
  displayUnit,
  value,
  index,
  isOverridden,
  disabled,
  disabledHint,
  onChange,
}: TierSliderProps) {
  const max = tiers.length - 1;
  const current = value || tiers[index];
  const currentDisplay = displayLabels[indexOf(current, tiers, index)] ?? current;
  return (
    <div>
      <div className="mb-2 flex items-baseline justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-1.5">
            <Label size="md" className="mb-0">{label}</Label>
            {disabled && (
              <Lock className="size-3.5 text-muted-foreground" aria-label="Locked" />
            )}
          </div>
          <p className="text-body-sm text-muted-foreground">
            {disabled && disabledHint ? disabledHint : description}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span className={cn(
            "text-heading-4 tabular-nums",
            isOverridden ? "text-foreground" : "text-muted-foreground",
          )}>
            {currentDisplay}
            {displayUnit && <span className="text-body-sm text-muted-foreground"> {displayUnit}</span>}
          </span>
          {isOverridden && !disabled && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`Reset ${label.toLowerCase()} to default`}
              onClick={() => onChange("")}
            >
              <RotateCcw />
            </Button>
          )}
        </div>
      </div>
      <Slider
        min={0}
        max={max}
        step={1}
        value={[index]}
        disabled={disabled}
        onValueChange={(values) => {
          const next = tiers[values[0]];
          if (next != null) onChange(next);
        }}
        aria-label={label}
      />
      <div className="mt-3 flex justify-between text-mono-sm text-faint-foreground">
        {displayLabels.map((l) => (
          <span key={l}>{l}</span>
        ))}
      </div>
    </div>
  );
}

function CostItem({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="text-mono-sm text-faint-foreground">{label}</div>
      <div className="text-body tabular-nums text-foreground">{formatUSD(value)}</div>
    </div>
  );
}
