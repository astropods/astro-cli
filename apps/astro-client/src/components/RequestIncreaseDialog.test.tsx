import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RequestIncreaseDialog } from "./RequestIncreaseDialog";

const mockMutate = vi.fn();
let mockError: unknown = null;

const mockReset = vi.fn();

vi.mock("@/api/queries/usage", () => ({
  useRequestQuotaIncrease: () => ({ mutate: mockMutate, reset: mockReset, isPending: false, error: mockError }),
}));

function renderDialog() {
  return render(
    <RequestIncreaseDialog
      featureKey="blueprints"
      label="Blueprints"
      meter={{ usage: 4, quota: 5 }}
      account="acme"
      open
      onOpenChange={() => {}}
    />,
  );
}

beforeEach(() => {
  mockMutate.mockReset();
  mockReset.mockReset();
  mockError = null;
});

describe("RequestIncreaseDialog error copy", () => {
  it("surfaces the server's own explanation over a generic Error.message", () => {
    mockError = {
      message: "Request failed with status 429",
      error_description: "You've already requested an increase this week.",
    };
    renderDialog();

    expect(screen.getByText("You've already requested an increase this week.")).toBeInTheDocument();
    expect(screen.queryByText("Request failed with status 429")).not.toBeInTheDocument();
  });

  it("falls back to friendly copy when the error carries no message", () => {
    mockError = {};
    renderDialog();

    expect(screen.getByText("Couldn't submit the request.")).toBeInTheDocument();
  });

  it("shows nothing until a request actually fails", () => {
    renderDialog();
    expect(screen.queryByText(/Couldn't submit|Request failed/)).not.toBeInTheDocument();
  });
});

describe("RequestIncreaseDialog reason validation", () => {
  it("keeps Submit enabled on a blank reason, so the error stays reachable", () => {
    renderDialog();
    expect(screen.getByRole("button", { name: "Submit request" })).not.toBeDisabled();
    expect(screen.queryByText("A reason is required.")).not.toBeInTheDocument();
  });

  it("shows an inline error instead of submitting when reason is blank", async () => {
    renderDialog();
    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));

    expect(screen.getByText("A reason is required.")).toBeInTheDocument();
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("clears the error and submits once a reason is entered", async () => {
    renderDialog();
    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));
    expect(screen.getByText("A reason is required.")).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText("Why do you need more quota?"), "Scaling up");
    expect(screen.queryByText("A reason is required.")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));
    expect(mockMutate).toHaveBeenCalledTimes(1);
  });

  it("doesn't resurface the stale error on the next open after a successful submit", async () => {
    renderDialog();
    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));
    expect(screen.getByText("A reason is required.")).toBeInTheDocument();

    await userEvent.type(screen.getByPlaceholderText("Why do you need more quota?"), "Scaling up");
    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));

    // The dialog stays mounted across opens in ResourceLimitsSection, so a
    // "touched" flag left set from the earlier blank submit would otherwise
    // resurface once the field it names is empty again.
    const onSuccess = mockMutate.mock.calls[0]![1].onSuccess;
    act(() => onSuccess());

    expect(screen.queryByText("A reason is required.")).not.toBeInTheDocument();
  });
});

function renderSpendLimitDialog() {
  return render(
    <RequestIncreaseDialog
      featureKey="spend_limit"
      label="Spend limit"
      meter={{ usage: 812.4, quota: 1000 }}
      account="acme"
      open
      onOpenChange={() => {}}
    />,
  );
}

describe("RequestIncreaseDialog for a spend limit", () => {
  it("reads the amounts as currency, not as a resource count", () => {
    renderSpendLimitDialog();

    expect(screen.getByText("$812.40")).toBeInTheDocument();
    expect(screen.getByText("$1,000.00")).toBeInTheDocument();
    expect(screen.getByText("Spend this period")).toBeInTheDocument();
    expect(screen.getByText("Current ceiling")).toBeInTheDocument();
  });

  it("requires an amount before it submits", async () => {
    renderSpendLimitDialog();
    await userEvent.type(screen.getByPlaceholderText("Why do you need more quota?"), "Batch run");
    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));

    expect(mockMutate).not.toHaveBeenCalled();
    expect(screen.getByText("An amount is required.")).toBeInTheDocument();
  });

  it("submits the amount under the spend-limit key", async () => {
    renderSpendLimitDialog();
    await userEvent.type(screen.getByPlaceholderText("0.00"), "5000");
    await userEvent.type(screen.getByPlaceholderText("Why do you need more quota?"), "Batch run");
    await userEvent.click(screen.getByRole("button", { name: "Submit request" }));

    expect(mockMutate).toHaveBeenCalledTimes(1);
    expect(mockMutate.mock.calls[0]![0]).toEqual({
      feature_key: "spend_limit",
      current_usage: 812.4,
      current_quota: 1000,
      requested_amount: 5000,
      reason: "Batch run",
    });
  });
});
