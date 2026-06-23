import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationHistoryDropdown } from "./ConversationHistoryDropdown";

describe("ConversationHistoryDropdown", () => {
  it("closes the history menu after selecting a conversation", async () => {
    const user = userEvent.setup();
    const onSelectSession = vi.fn();

    render(
      <ConversationHistoryDropdown
        sessions={[
          {
            conversationId: "conv-1",
            deploymentId: "dep-1",
            title: "First chat",
            updatedAt: "2026-06-22T12:00:00Z",
          },
        ]}
        activeConversationId={null}
        onSelectSession={onSelectSession}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Chat history" }));
    await waitFor(() => {
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("menuitem", { name: /First chat/i }));

    expect(onSelectSession).toHaveBeenCalledWith("conv-1");
    await waitFor(() => {
      expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    });
  });
});
