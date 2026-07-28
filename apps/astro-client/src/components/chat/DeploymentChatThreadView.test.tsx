import { useState } from "react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
import { ApiClientProvider } from "@/lib/api-context";
import type { ApiClient } from "@/lib/api";
import type { Interaction } from "@/lib/chat/interaction";

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
  // The composer's attach/drag-drop upload uses TanStack Query, so the view
  // needs a QueryClient in scope.
  const [queryClient] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  const runtime = useExternalStoreRuntime({
    messages,
    isRunning,
    onNew: async () => {},
    onCancel: onCancel ? async () => onCancel() : undefined,
    convertMessage: (m) => m,
  });
  return (
    <QueryClientProvider client={queryClient}>
      <AssistantRuntimeProvider runtime={runtime}>
        <DeploymentChatThreadView
          account="acme"
          deploymentId="dep-1"
          agentLabel="Test Agent"
          composerState={composerState}
        />
      </AssistantRuntimeProvider>
    </QueryClientProvider>
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
    filesEnabled: false,
    pendingInteraction: null,
    clearPendingInteraction: () => {},
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

// Zero-field tool_permission, so "Approve" submits an empty content object.
const toolPermission: Interaction = {
  id: "int-1",
  kind: "form",
  message: "",
  intent: "tool_permission",
  dataSchema: { type: "object", title: "delete_files", properties: {} },
  actions: ["submit", "decline"],
};

function InteractionHarness({
  pendingInteraction,
  clearPendingInteraction = () => {},
  respondToInteraction = vi.fn().mockResolvedValue({ status: "ok", action: "submit" }),
}: {
  pendingInteraction: Interaction | null;
  clearPendingInteraction?: () => void;
  respondToInteraction?: ReturnType<typeof vi.fn>;
}) {
  const [queryClient] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  const runtime = useExternalStoreRuntime({
    messages: NO_MESSAGES,
    isRunning: false,
    onNew: async () => {},
    convertMessage: (m) => m,
  });
  const viewport: DeploymentChatViewportState = {
    streamingMessageId: null,
    conversationId: "conv-1",
    historyLoading: false,
    isStreaming: false,
    streamError: null,
    hasMoreHistory: false,
    loadOlderMessages: async () => {},
    filesEnabled: false,
    pendingInteraction,
    clearPendingInteraction,
  };
  const api = { respondToInteraction } as unknown as ApiClient;
  return (
    <QueryClientProvider client={queryClient}>
      <ApiClientProvider value={api}>
        <AssistantRuntimeProvider runtime={runtime}>
          <DeploymentChatStreamingContext.Provider value={viewport}>
            <DeploymentChatThreadView
              account="acme"
              deploymentId="dep-1"
              agentLabel="Test Agent"
              composerState="ready"
            />
          </DeploymentChatStreamingContext.Provider>
        </AssistantRuntimeProvider>
      </ApiClientProvider>
    </QueryClientProvider>
  );
}

describe("DeploymentChatThreadView interaction composer", () => {
  it("shows the normal composer when nothing is pending", () => {
    render(<InteractionHarness pendingInteraction={null} />);
    expect(screen.getByLabelText("Message input")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
  });

  it("replaces the composer with the interaction form while one is pending", () => {
    render(<InteractionHarness pendingInteraction={toolPermission} />);
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Message input")).not.toBeInTheDocument();
  });

  it("resets form state when the pending interaction advances", () => {
    const a: Interaction = {
      id: "a",
      kind: "form",
      message: "",
      dataSchema: { type: "object", properties: { name: { type: "string" } }, required: ["name"] },
      actions: ["submit"],
    };
    const b: Interaction = {
      id: "b",
      kind: "form",
      message: "",
      dataSchema: { type: "object", properties: { email: { type: "string" } }, required: ["email"] },
      actions: ["submit"],
    };
    const { rerender } = render(<InteractionHarness pendingInteraction={a} />);

    // Force A into a validation-error state.
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(screen.getByText("Required.")).toBeInTheDocument();

    // Advancing the FIFO queue to B must not carry A's error state over.
    rerender(<InteractionHarness pendingInteraction={b} />);
    expect(screen.queryByText("Required.")).not.toBeInTheDocument();
  });

  it("submits the response and clears the pending interaction on success", async () => {
    const clearPendingInteraction = vi.fn();
    const respondToInteraction = vi
      .fn()
      .mockResolvedValue({ status: "ok", action: "submit" });
    render(
      <InteractionHarness
        pendingInteraction={toolPermission}
        clearPendingInteraction={clearPendingInteraction}
        respondToInteraction={respondToInteraction}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() =>
      expect(respondToInteraction).toHaveBeenCalledWith("dep-1", "conv-1", "int-1", {
        action: "submit",
        content: {},
      }),
    );
    await waitFor(() => expect(clearPendingInteraction).toHaveBeenCalledTimes(1));
  });
});
