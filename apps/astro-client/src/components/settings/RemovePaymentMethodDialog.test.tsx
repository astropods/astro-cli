import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RemovePaymentMethodDialog } from "./RemovePaymentMethodDialog";

const mockMutate = vi.fn();
const mockReset = vi.fn();
const mockDeleteState = vi.fn();

vi.mock("@/api/queries", () => ({
  useDeletePaymentMethod: () => mockDeleteState(),
}));

function renderDialog(onUpdateCard = vi.fn()) {
  const onOpenChange = vi.fn();
  render(
    <RemovePaymentMethodDialog
      account="acme"
      open
      onOpenChange={onOpenChange}
      onUpdateCard={onUpdateCard}
    />,
  );
  return { onOpenChange, onUpdateCard };
}

async function checkAndConfirm() {
  await userEvent.click(screen.getByRole("checkbox"));
  await userEvent.click(screen.getByRole("button", { name: "Remove payment method" }));
}

beforeEach(() => {
  mockMutate.mockReset();
  mockReset.mockReset();
  mockDeleteState.mockReset().mockReturnValue({
    mutate: mockMutate,
    reset: mockReset,
    isPending: false,
    isError: false,
    error: null,
  });
});

describe("RemovePaymentMethodDialog", () => {
  it("requires the destructive checkbox before the confirm button does anything", async () => {
    renderDialog();

    await userEvent.click(screen.getByRole("button", { name: "Remove payment method" }));

    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("removes the card once confirmed", async () => {
    renderDialog();

    await checkAndConfirm();

    expect(mockMutate).toHaveBeenCalledTimes(1);
  });

  it("closes the dialog once removal succeeds", async () => {
    const { onOpenChange } = renderDialog();
    mockMutate.mockImplementation((_arg, opts) => opts.onSuccess());

    await checkAndConfirm();

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("opens the update-card flow and closes this dialog, in that order", async () => {
    const { onOpenChange, onUpdateCard } = renderDialog();

    await userEvent.click(screen.getByRole("button", { name: "Update card" }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onUpdateCard).toHaveBeenCalled();
    const closeOrder = onOpenChange.mock.invocationCallOrder[0];
    const updateOrder = onUpdateCard.mock.invocationCallOrder[0];
    expect(closeOrder).toBeLessThan(updateOrder);
  });

  it("shows the mutation's own error message, not the fallback, when one exists", async () => {
    mockDeleteState.mockReturnValue({
      mutate: mockMutate,
      reset: mockReset,
      isPending: false,
      isError: true,
      error: new Error("card is still on an open invoice"),
    });
    renderDialog();

    expect(screen.getByText("card is still on an open invoice")).toBeInTheDocument();
  });

  it("falls back to a generic message when the mutation error has none", async () => {
    mockDeleteState.mockReturnValue({
      mutate: mockMutate,
      reset: mockReset,
      isPending: false,
      isError: true,
      error: new Error(""),
    });
    renderDialog();

    expect(screen.getByText("Failed to remove payment method.")).toBeInTheDocument();
  });

  it("resets the mutation state on cancel, so a stale error doesn't linger", async () => {
    renderDialog();

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(mockReset).toHaveBeenCalledTimes(1);
    expect(mockMutate).not.toHaveBeenCalled();
  });
});
