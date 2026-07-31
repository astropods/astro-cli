import type { Meta, StoryObj } from "@storybook/react-vite";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/toaster";

function DemoButton(props: React.ComponentProps<typeof Button>) {
  return <Button variant="outline" size="sm" {...props} />;
}

function ToastDemo() {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <DemoButton onClick={() => toast("Trace added to dataset")}>Default</DemoButton>
      <DemoButton onClick={() => toast.success("Criteria saved")}>Success</DemoButton>
      <DemoButton
        onClick={() =>
          toast.warning("Some traces couldn’t be judged", {
            description: "Retry them on the next run or select a verdict.",
          })
        }
      >
        Warning
      </DemoButton>
      <DemoButton onClick={() => toast.error("Could not save criteria. Try again.")}>
        Error
      </DemoButton>
      <DemoButton onClick={() => toast.info("Reindexing started")}>Info</DemoButton>
      <DemoButton
        onClick={() =>
          toast.success("Dataset updated", {
            description: "3 traces were added to the evaluation dataset.",
          })
        }
      >
        With description
      </DemoButton>
      <DemoButton
        onClick={() =>
          toast("Trace removed", {
            action: { label: "Undo", onClick: () => toast("Restored") },
          })
        }
      >
        With action
      </DemoButton>
      <Toaster />
    </div>
  );
}

const meta = {
  title: "UI/Toast",
  component: ToastDemo,
  parameters: { layout: "centered" },
} satisfies Meta<typeof ToastDemo>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
