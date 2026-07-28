import { describe, it, expect, vi, afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { InteractionForm } from "./InteractionForm";
import type { Interaction } from "./types";

afterEach(cleanup);

function textInteraction(overrides: Partial<Interaction> = {}): Interaction {
  return {
    id: "i1",
    kind: "form",
    message: "What's your GitHub username?",
    dataSchema: { type: "object", properties: { username: { type: "string" } }, required: ["username"] },
    actions: ["submit", "cancel"],
    ...overrides,
  };
}

describe("<InteractionForm>", () => {
  it("submits the entered content", async () => {
    const onSubmit = vi.fn();
    render(<InteractionForm interaction={textInteraction()} onSubmit={onSubmit} onCancel={vi.fn()} />);

    await userEvent.type(screen.getByRole("textbox"), "octocat");
    await userEvent.click(screen.getByRole("button", { name: "Submit" }));

    expect(onSubmit).toHaveBeenCalledWith({ username: "octocat" });
  });

  it("submits native numeric enum values, not strings", async () => {
    const onSubmit = vi.fn();
    const interaction: Interaction = {
      id: "n1",
      kind: "form",
      message: "Pick counts",
      dataSchema: {
        type: "object",
        properties: { counts: { type: "array", items: { type: "integer", enum: [1, 2, 3] } } },
        required: ["counts"],
      },
      actions: ["submit"],
    };
    render(<InteractionForm interaction={interaction} onSubmit={onSubmit} onCancel={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "1" }));
    await userEvent.click(screen.getByRole("button", { name: "Submit" }));

    expect(onSubmit).toHaveBeenCalledWith({ counts: [1] });
  });

  it("gates submit on required fields and shows an error", async () => {
    const onSubmit = vi.fn();
    render(<InteractionForm interaction={textInteraction()} onSubmit={onSubmit} onCancel={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "Submit" }));

    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText("Required.")).toBeInTheDocument();
  });

  it("fires cancel", async () => {
    const onCancel = vi.fn();
    render(<InteractionForm interaction={textInteraction()} onSubmit={vi.fn()} onCancel={onCancel} />);
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });

  it("captures a free-text reply via respond mode", async () => {
    const onRespond = vi.fn();
    render(
      <InteractionForm
        interaction={textInteraction({ actions: ["submit", "respond", "cancel"] })}
        onSubmit={vi.fn()}
        onRespond={onRespond}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /write your own reply/i }));
    await userEvent.type(screen.getByPlaceholderText("Type your answer…"), "release/2026-07");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(onRespond).toHaveBeenCalledWith("release/2026-07");
  });

  it("shows an inline error instead of disabling Send on an empty reply", async () => {
    const onRespond = vi.fn();
    render(
      <InteractionForm
        interaction={textInteraction({ actions: ["submit", "respond", "cancel"] })}
        onSubmit={vi.fn()}
        onRespond={onRespond}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /write your own reply/i }));
    const send = screen.getByRole("button", { name: "Send" });
    expect(send).toBeEnabled();

    await userEvent.click(send);
    expect(onRespond).not.toHaveBeenCalled();
    expect(screen.getByText("Enter a reply.")).toBeInTheDocument();
  });

  it("renders a tool_permission gate with the humanized tool name and approve/deny", () => {
    render(
      <InteractionForm
        interaction={{
          id: "i1",
          kind: "form",
          message: "",
          intent: "tool_permission",
          dataSchema: {
            type: "object",
            title: "bash",
            description: "LLM-facing, must not render",
            properties: { command: { type: "string", "x-ui": { widget: "code" } } },
            required: ["command"],
          },
          value: { command: "ls -la" },
          actions: ["submit", "decline"],
        }}
        onSubmit={vi.fn()}
        onDecline={vi.fn()}
      />,
    );

    expect(screen.getByText("Permission required")).toBeInTheDocument();
    expect(screen.getByText("Bash")).toBeInTheDocument();
    // The LLM-facing tool description is never shown.
    expect(screen.queryByText("LLM-facing, must not render")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deny" })).toBeInTheDocument();
  });

  it("disables actions while pending", () => {
    render(<InteractionForm interaction={textInteraction()} pending onSubmit={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });
});
