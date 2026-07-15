import { useState } from "react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import {
  AssistantRuntimeProvider,
  useComposer,
  useExternalStoreRuntime,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { DeploymentChatThreadView } from "./DeploymentChatThreadView";
import {
  DeploymentChatStreamingContext,
  type DeploymentChatViewportState,
} from "./deployment-chat-streaming-context";
import type { ChatComposerState } from "@/lib/deployment-utils";

// jsdom lacks Element.scrollTo, which assistant-ui calls during auto-scroll.
beforeAll(() => {
  if (!Element.prototype.scrollTo) {
    Element.prototype.scrollTo = () => {};
  }
});

// Stable empty array so re-renders don't feed useExternalStoreRuntime a new
// messages reference (which would confound the draft-persistence assertions).
const NO_MESSAGES: ThreadMessageLike[] = [];

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

// The runtime persists across conversation switches (ChatThread is keyed on
// deploymentId), so the composer store outlives a switch. DraftHarness holds one
// runtime and flips conversationId via internal state, reproducing the switch
// without remounting the runtime. The probe reads the live composer text.
function ComposerTextProbe() {
  const text = useComposer((c) => c.text);
  return <span data-testid="composer-text">{text}</span>;
}

function DraftHarness({ initialConversationId = "conv-a" as string | null }) {
  const [conversationId, setConversationId] = useState<string | null>(
    initialConversationId,
  );
  const runtime = useExternalStoreRuntime({
    messages: NO_MESSAGES,
    isRunning: false,
    onNew: async () => {},
    convertMessage: (m) => m,
  });
  const viewport: DeploymentChatViewportState = {
    streamingMessageId: null,
    conversationId,
    historyLoading: false,
    isStreaming: false,
    streamError: null,
    hasMoreHistory: false,
    loadOlderMessages: async () => {},
  };
  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <DeploymentChatStreamingContext.Provider value={viewport}>
        <button onClick={() => setConversationId("conv-a")}>to-a</button>
        <button onClick={() => setConversationId("conv-b")}>to-b</button>
        <button onClick={() => setConversationId(conversationId)}>rerender</button>
        <ComposerTextProbe />
        <DeploymentChatThreadView
          account="acme"
          deploymentId="dep-1"
          agentLabel="Test Agent"
          composerState="ready"
        />
      </DeploymentChatStreamingContext.Provider>
    </AssistantRuntimeProvider>
  );
}

const input = () =>
  screen.getByLabelText("Message input") as HTMLTextAreaElement;
const composerText = () => screen.getByTestId("composer-text").textContent;

describe("DeploymentChatThreadView composer draft", () => {
  beforeEach(() => sessionStorage.clear());

  it("does not leak a draft into another conversation", () => {
    render(<DraftHarness />);
    fireEvent.change(input(), { target: { value: "draft for A" } });
    expect(composerText()).toBe("draft for A");

    fireEvent.click(screen.getByText("to-b"));
    expect(composerText()).toBe("");
  });

  it("restores a per-conversation draft when returning to it", () => {
    render(<DraftHarness />);
    fireEvent.change(input(), { target: { value: "draft for A" } });

    fireEvent.click(screen.getByText("to-b"));
    fireEvent.change(input(), { target: { value: "draft for B" } });
    expect(composerText()).toBe("draft for B");

    fireEvent.click(screen.getByText("to-a"));
    expect(composerText()).toBe("draft for A");
    fireEvent.click(screen.getByText("to-b"));
    expect(composerText()).toBe("draft for B");
  });

  it("does not resurrect a sent/cleared draft", () => {
    render(<DraftHarness />);
    fireEvent.change(input(), { target: { value: "draft for A" } });
    // Simulate the composer clearing on send (conversation is unchanged).
    fireEvent.change(input(), { target: { value: "" } });

    fireEvent.click(screen.getByText("to-b"));
    fireEvent.click(screen.getByText("to-a"));
    expect(composerText()).toBe("");
  });

  it("persists a draft across a reload (remount with a fresh runtime)", () => {
    const { unmount } = render(<DraftHarness />);
    fireEvent.change(input(), { target: { value: "survives reload" } });
    unmount();

    // A remount builds a brand-new runtime (empty composer); the draft comes
    // back from sessionStorage for the same conversation.
    render(<DraftHarness />);
    expect(composerText()).toBe("survives reload");
  });

  it("keeps the draft when the conversation is unchanged (no clear on re-render)", () => {
    render(<DraftHarness />);
    fireEvent.change(input(), { target: { value: "keep me" } });
    fireEvent.click(screen.getByText("rerender"));
    expect(composerText()).toBe("keep me");
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
