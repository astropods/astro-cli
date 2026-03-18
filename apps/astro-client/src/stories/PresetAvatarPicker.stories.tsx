import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { PresetAvatarPicker } from "@/components/PresetAvatarPicker";
import { PRESET_AVATARS } from "@/lib/presetAvatars";

const meta = {
  title: "Design System/Composites/PresetAvatarPicker",
  component: PresetAvatarPicker,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof PresetAvatarPicker>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | null>(null);
    return (
      <div className="w-80">
        <PresetAvatarPicker value={selected} onChange={setSelected} />
        {selected && (
          <p className="mt-3 text-body-sm text-muted-foreground">
            Selected: <code className="font-mono">{selected}</code>
          </p>
        )}
      </div>
    );
  },
  args: { value: null, onChange: () => {} },
};

export const WithSelection: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | null>("avatar_03");
    return (
      <div className="w-80">
        <PresetAvatarPicker value={selected} onChange={setSelected} />
      </div>
    );
  },
  args: { value: null, onChange: () => {} },
};

export const AllAvatars: Story = {
  render: () => (
    <div className="p-4">
      <p className="text-label text-muted-foreground mb-4 font-mono uppercase tracking-wide">
        {PRESET_AVATARS.length} preset avatars
      </p>
      <div className="grid grid-cols-5 gap-3">
        {PRESET_AVATARS.map((avatar) => (
          <div key={avatar.id} className="flex flex-col items-center gap-1.5">
            <img
              src={avatar.src}
              alt={avatar.label}
              className="w-16 aspect-square rounded-lg object-cover"
            />
            <span className="text-mono-sm font-mono text-faint-foreground">
              {avatar.id}
            </span>
          </div>
        ))}
      </div>
    </div>
  ),
  args: { value: null, onChange: () => {} },
};
