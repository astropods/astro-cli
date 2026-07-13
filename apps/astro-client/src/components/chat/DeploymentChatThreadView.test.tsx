import { beforeAll, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

// jsdom lacks Element.scrollTo, which assistant-ui calls during auto-scroll.
beforeAll(() => {
  if (!Element.prototype.scrollTo) {
    Element.prototype.scrollTo = () => {};
  }
});
import {
  AssistantRuntimeProvider,
  useExternalStoreRuntime,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { DeploymentChatThreadView } from "./DeploymentChatThreadView";
import type { ChatComposerState } from "@/lib/deployment-utils";

function Harness({
  messages,
  isRunning = false,
  onCancel,
  composerState = "ready",
}: {
  messages: ThreadMessageLike[];
  isRunning?: boolean;
  onCancel?: () => void;
  composerState?: ChatComposerState;
}) {
  const runtime = useExternalStoreRuntime({
    messages,
    isRunning,
    onNew: async () => {},
    onCancel: onCancel ? async () => onCancel() : undefined,
    convertMessage: (m) => m,
  });
  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <DeploymentChatThreadView
        account="acme"
        deploymentId="dep-1"
        agentLabel="Test Agent"
        composerState={composerState}
      />
    </AssistantRuntimeProvider>
  );
}

describe("DeploymentChatThreadView empty state", () => {
  it("shows the agent-scoped welcome prompt when the thread is empty", () => {
    render(<Harness messages={[]} />);
    expect(
      screen.getByRole("heading", { name: "What should Test Agent work on?" }),
    ).toBeInTheDocument();
  });

  it("enlarges the composer input only while the thread is empty", () => {
    render(<Harness messages={[]} />);
    expect(screen.getByLabelText("Message input").className).toContain("min-h-22");
  });

  it("uses the standard composer height once the thread has messages", async () => {
    render(<Harness messages={[{ id: "m1", role: "user", content: "hello" }]} />);

    await waitFor(() => {
      const input = screen.getByLabelText("Message input");
      expect(input.className).toContain("min-h-8");
      expect(input.className).not.toContain("min-h-22");
    });
    expect(
      screen.queryByRole("heading", { name: "What should Test Agent work on?" }),
    ).not.toBeInTheDocument();
  });
});

describe("DeploymentChatThreadView composer controls", () => {
  it("auto-focuses the composer input when the agent is ready", async () => {
    render(<Harness messages={[]} />);
    await waitFor(() => {
      expect(screen.getByLabelText("Message input")).toHaveFocus();
    });
  });

  it("does not auto-focus the composer while the agent is not ready", async () => {
    render(<Harness messages={[]} composerState="starting" />);
    // Give any focus effect a chance to run before asserting it did not fire.
    await waitFor(() => {
      expect(screen.getByLabelText("Message input")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Message input")).not.toHaveFocus();
  });

  // Esc-to-stop is library behavior; guard it stays wired through our runtime.
  it("cancels an in-flight turn when Escape is pressed", async () => {
    const onCancel = vi.fn();
    render(
      <Harness
        messages={[{ id: "m1", role: "user", content: "hello" }]}
        isRunning
        onCancel={onCancel}
      />,
    );
    fireEvent.keyDown(screen.getByLabelText("Message input"), { key: "Escape" });
    await waitFor(() => expect(onCancel).toHaveBeenCalled());
  });
});
