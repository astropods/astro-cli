import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  MultiSelect,
  MultiSelectTrigger,
  MultiSelectValue,
  MultiSelectContent,
  MultiSelectList,
  MultiSelectAllItem,
  MultiSelectClearItem,
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

const CATEGORY_OPTIONS: MultiSelectOption[] = [
  { value: "analytics", label: "Analytics" },
  { value: "coding", label: "Coding" },
  { value: "github", label: "GitHub" },
  { value: "google-workspace", label: "Google Workspace" },
  { value: "issues", label: "Issues" },
  { value: "knowledge-graph", label: "Knowledge Graph" },
  { value: "mcp", label: "MCP" },
  { value: "productivity", label: "Productivity" },
  { value: "scheduling", label: "Scheduling" },
  { value: "workspace", label: "Workspace" },
  { value: "whatsapp", label: "WhatsApp" },
];

function MultiSelectDemo({
  options,
  placeholder,
  initialValue = [],
  contentClassName,
  listClassName,
}: {
  options: MultiSelectOption[];
  placeholder?: string;
  initialValue?: string[];
  contentClassName?: string;
  listClassName?: string;
}) {
  const [value, setValue] = useState<string[]>(initialValue);
  return (
    <div className="w-[220px]">
      <MultiSelect value={value} onValueChange={setValue}>
        <MultiSelectTrigger>
          <MultiSelectValue placeholder={placeholder} options={options} />
        </MultiSelectTrigger>
        <MultiSelectContent className={contentClassName}>
          <MultiSelectAllItem>{placeholder ?? "All"}</MultiSelectAllItem>
          <MultiSelectList className={listClassName}>
            {options.map((o) => (
              <MultiSelectItem key={o.value} value={o.value} color={o.color}>
                {o.label}
              </MultiSelectItem>
            ))}
          </MultiSelectList>
          <MultiSelectClearItem />
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

export const Default: Story = {
  args: { options: STATUS_OPTIONS, placeholder: "All statuses" },
};

export const SingleSelected: Story = {
  args: {
    options: STATUS_OPTIONS,
    placeholder: "All statuses",
    initialValue: ["success"],
  },
};

export const MultipleSelected: Story = {
  args: {
    options: STATUS_OPTIONS,
    placeholder: "All statuses",
    initialValue: ["success", "error"],
  },
};

export const ScrollableList: Story = {
  args: {
    options: CATEGORY_OPTIONS,
    placeholder: "Filter",
    initialValue: ["productivity", "mcp"],
    contentClassName: "w-64",
    listClassName: "max-h-40",
  },
};

export const WithoutColorIndicators: Story = {
  args: {
    options: MODEL_OPTIONS,
    placeholder: "All models",
  },
};
