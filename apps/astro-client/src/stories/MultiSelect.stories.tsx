import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  MultiSelect,
  MultiSelectTrigger,
  MultiSelectValue,
  MultiSelectContent,
  MultiSelectAllItem,
  MultiSelectItem,
  type MultiSelectOption,
} from "@/components/ui/multi-select";

const STATUS_OPTIONS: MultiSelectOption[] = [
  { value: "success", label: "Success", color: "var(--color-green-700)" },
  { value: "error", label: "Error", color: "var(--color-coral-600)" },
  { value: "timeout", label: "Timeout", color: "var(--color-yellow-500)" },
];

const MODEL_OPTIONS: MultiSelectOption[] = [
  { value: "claude-opus", label: "Claude Opus 4.6" },
  { value: "claude-sonnet", label: "Claude Sonnet 4.6" },
  { value: "claude-haiku", label: "Claude Haiku 4.5" },
  { value: "gpt-4o", label: "GPT-4o" },
];

function MultiSelectDemo({
  options,
  placeholder,
  initialValue = [],
}: {
  options: MultiSelectOption[];
  placeholder?: string;
  initialValue?: string[];
}) {
  const [value, setValue] = useState<string[]>(initialValue);
  return (
    <div className="w-[220px]">
      <MultiSelect value={value} onValueChange={setValue}>
        <MultiSelectTrigger>
          <MultiSelectValue placeholder={placeholder} options={options} />
        </MultiSelectTrigger>
        <MultiSelectContent>
          <MultiSelectAllItem>{placeholder ?? "All"}</MultiSelectAllItem>
          {options.map((o) => (
            <MultiSelectItem key={o.value} value={o.value} color={o.color}>
              {o.label}
            </MultiSelectItem>
          ))}
        </MultiSelectContent>
      </MultiSelect>
    </div>
  );
}

const meta = {
  title: "Design System/Primitives/MultiSelect",
  component: MultiSelectDemo,
} satisfies Meta<typeof MultiSelectDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllStates: Story = {
  args: { options: STATUS_OPTIONS, placeholder: "All statuses" },
  render: () => (
    <div className="flex flex-col gap-6 max-w-xs">
      <div className="flex flex-col gap-1">
        <MultiSelectDemo options={STATUS_OPTIONS} placeholder="All statuses" />
        <span className="text-mono-sm text-muted-foreground">Empty (all selected)</span>
      </div>
      <div className="flex flex-col gap-1">
        <MultiSelectDemo
          options={STATUS_OPTIONS}
          placeholder="All statuses"
          initialValue={["success"]}
        />
        <span className="text-mono-sm text-muted-foreground">Single selected</span>
      </div>
      <div className="flex flex-col gap-1">
        <MultiSelectDemo
          options={STATUS_OPTIONS}
          placeholder="All statuses"
          initialValue={["success", "error"]}
        />
        <span className="text-mono-sm text-muted-foreground">Multiple selected</span>
      </div>
      <div className="flex flex-col gap-1">
        <MultiSelectDemo options={MODEL_OPTIONS} placeholder="All models" />
        <span className="text-mono-sm text-muted-foreground">Without color indicators</span>
      </div>
    </div>
  ),
};
