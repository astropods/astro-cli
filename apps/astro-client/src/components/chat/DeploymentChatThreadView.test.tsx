import { describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import {
  AssistantRuntimeProvider,
  useExternalStoreRuntime,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { DeploymentChatThreadView } from "./DeploymentChatThreadView";

function Harness({ messages }: { messages: ThreadMessageLike[] }) {
  const runtime = useExternalStoreRuntime({
    messages,
    isRunning: false,
    onNew: async () => {},
    convertMessage: (m) => m,
  });
  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <DeploymentChatThreadView
        account="acme"
        deploymentId="dep-1"
        agentLabel="Test Agent"
        composerState="ready"
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
