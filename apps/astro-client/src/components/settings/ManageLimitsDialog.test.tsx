import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { BillingSpend } from "@/lib/api";
import { buildSpendResponse } from "@/api/queries/billing.fixtures";
import { ManageLimitsDialog } from "./ManageLimitsDialog";

const mockSpend = vi.fn();
const mockRole = vi.fn<() => string | null>(() => "owner");
const mockPersonalAccount = vi.fn<() => { name: string } | undefined>(() => undefined);
const mockMutateAsync = vi.fn().mockResolvedValue({ available: true });
const mockToastSuccess = vi.fn();
const mockToastWarning = vi.fn();
const mockToastError = vi.fn();

vi.mock("sonner", () => ({
  toast: {
    success: (m: string) => mockToastSuccess(m),
    warning: (m: string) => mockToastWarning(m),
    error: (m: string) => mockToastError(m),
  },
}));
vi.mock("@/lib/auth", () => ({ useAuth: () => ({ role: mockRole(), personalAccount: mockPersonalAccount() }) }));
vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: (account: string) => mockSpend(account),
  useSetBillingSpendThresholds: () => ({ mutateAsync: mockMutateAsync, isPending: false }),
}));

function spendResponse(partial: Partial<BillingSpend> = {}) {
  return {
    data: buildSpendResponse({
      current_spend: 45.02,
      usage_spend: 45.02,
      current_period_end: "2026-08-23T12:00:00Z",
      ...partial,
    }),
  };
}

function renderDialog() {
  return render(<ManageLimitsDialog account="acme" open onOpenChange={() => {}} />);
}

beforeEach(() => {
  mockSpend.mockReset().mockReturnValue(spendResponse());
  mockRole.mockReset().mockReturnValue("owner");
  mockPersonalAccount.mockReset().mockReturnValue(undefined);
  mockMutateAsync.mockReset().mockResolvedValue({ available: true });
  mockToastSuccess.mockClear();
  mockToastWarning.mockClear();
  mockToastError.mockClear();
});

describe("ManageLimitsDialog reading stored thresholds", () => {
  it("converts the stored cents amount to a dollar figure in the field", () => {
    mockSpend.mockReturnValue(
      spendResponse({ warning: { amount: 2500, in_alarm: false }, limit: { amount: 3000, in_alarm: false } }),
    );
    renderDialog();

    expect(screen.getByLabelText("Alert threshold")).toHaveValue("25");
    expect(screen.getByLabelText("Spend limit")).toHaveValue("30");
  });
});

describe("ManageLimitsDialog opened against a cold cache", () => {
  it("keeps what the account typed once the first load lands", async () => {
    mockSpend.mockReturnValue({ data: undefined, isLoading: true });
    const { rerender } = renderDialog();

    await userEvent.type(screen.getByLabelText("Spend limit"), "42");

    mockSpend.mockReturnValue({ ...spendResponse({ limit: { amount: 3000, in_alarm: false } }), isLoading: false });
    rerender(<ManageLimitsDialog account="acme" open onOpenChange={() => {}} />);

    expect(screen.getByLabelText("Spend limit")).toHaveValue("42");
  });
});

describe("ManageLimitsDialog validation", () => {
  it("has no upper bound on the spend limit", async () => {
    renderDialog();
    await userEvent.clear(screen.getByLabelText("Spend limit"));
    await userEvent.type(screen.getByLabelText("Spend limit"), "50000");
    await userEvent.click(screen.getByRole("button", { name: "Save limits" }));

    expect(mockMutateAsync).toHaveBeenCalledWith({ warning: null, limit: 5_000_000 });
  });

  it("blocks an alert set at or above the limit, but keeps Save enabled so the error stays reachable", async () => {
    renderDialog();
    await userEvent.clear(screen.getByLabelText("Spend limit"));
    await userEvent.type(screen.getByLabelText("Spend limit"), "50");
    await userEvent.clear(screen.getByLabelText("Alert threshold"));
    await userEvent.type(screen.getByLabelText("Alert threshold"), "95");

    expect(
      screen.getByText(/Agents already pause at your \$50\.00 spend limit/),
    ).toBeInTheDocument();

    // A disabled button drops out of tab order, so a keyboard/screen-reader
    // user could never reach it to discover why; onSave's own early return
    // on blockingError is what actually guards the write.
    const save = screen.getByRole("button", { name: "Save limits" });
    expect(save).not.toBeDisabled();

    await userEvent.click(save);
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  it("warns without blocking when the limit is already below this period's spend", async () => {
    renderDialog();
    await userEvent.clear(screen.getByLabelText("Spend limit"));
    await userEvent.type(screen.getByLabelText("Spend limit"), "30");

    expect(screen.getByText(/Spend this period is already \$45\.02/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save limits" })).not.toBeDisabled();
  });
});

describe("ManageLimitsDialog Save button", () => {
  it("disables on open, before anything has changed", () => {
    renderDialog();
    expect(screen.getByRole("button", { name: "Save limits" })).toBeDisabled();
  });

  it("enables once a field changes, and disables again once it's reverted", async () => {
    renderDialog();
    const save = screen.getByRole("button", { name: "Save limits" });
    const spendLimit = screen.getByLabelText("Spend limit");

    await userEvent.type(spendLimit, "60");
    expect(save).not.toBeDisabled();

    await userEvent.clear(spendLimit);
    expect(save).toBeDisabled();
  });
});

describe("ManageLimitsDialog saving", () => {
  it("writes both fields in cents", async () => {
    renderDialog();
    await userEvent.clear(screen.getByLabelText("Alert threshold"));
    await userEvent.type(screen.getByLabelText("Alert threshold"), "10");
    await userEvent.clear(screen.getByLabelText("Spend limit"));
    await userEvent.type(screen.getByLabelText("Spend limit"), "60");
    await userEvent.click(screen.getByRole("button", { name: "Save limits" }));

    expect(mockMutateAsync).toHaveBeenCalledWith({ warning: 1000, limit: 6000 });
  });

  it("shows the server's error message when the write fails", async () => {
    mockMutateAsync.mockRejectedValue({ error: "provider unavailable" });
    renderDialog();
    await userEvent.clear(screen.getByLabelText("Spend limit"));
    await userEvent.type(screen.getByLabelText("Spend limit"), "60");
    await userEvent.click(screen.getByRole("button", { name: "Save limits" }));

    await vi.waitFor(() => expect(mockToastError).toHaveBeenCalledWith("provider unavailable"));
  });
});

describe("ManageLimitsDialog without manage permission", () => {
  it("disables the fields and explains why", () => {
    mockRole.mockReturnValue("member");
    renderDialog();

    expect(screen.getByLabelText("Alert threshold")).toBeDisabled();
    expect(screen.getByText("Only owners and admins can change limits.")).toBeInTheDocument();
  });
});

describe("ManageLimitsDialog on the account's own personal account", () => {
  // `role` is null on a personal account; canManageBilling(role) alone would deny its owner.
  it("enables the fields for the owner regardless of the session's org role", () => {
    mockRole.mockReturnValue(null);
    mockPersonalAccount.mockReturnValue({ name: "acme" });
    renderDialog();

    expect(screen.getByLabelText("Alert threshold")).not.toBeDisabled();
    expect(screen.queryByText("Only owners and admins can change limits.")).not.toBeInTheDocument();
  });
});

describe("ManageLimitsDialog when the write succeeds but the limit lift fails", () => {
  it("warns instead of claiming plain success", async () => {
    mockMutateAsync.mockResolvedValue({ available: true, limit_lift_failed: true });
    renderDialog();
    await userEvent.clear(screen.getByLabelText("Spend limit"));
    await userEvent.type(screen.getByLabelText("Spend limit"), "60");
    await userEvent.click(screen.getByRole("button", { name: "Save limits" }));

    expect(mockToastWarning).toHaveBeenCalledWith(
      "Limits saved, but agents are still paused. Try raising the limit again in a moment.",
    );
    expect(mockToastSuccess).not.toHaveBeenCalled();
  });
});

describe("ManageLimitsDialog has no per-metric controls", () => {
  // Product's design shows one account-wide spend cap; Compute and AI
  // Gateway thresholds aren't self-serve here even though the account may
  // already hold one (see the changelog for why).
  it("doesn't show Compute or AI Gateway fields even when the account has thresholds set on them", () => {
    mockSpend.mockReturnValue(
      spendResponse({
        usage: {
          compute: { unit: "CU-hours", warning: { amount: 100, in_alarm: false }, limit: { amount: 200, in_alarm: true } },
          gateway: { unit: "USD of model usage", warning: { amount: 10, in_alarm: false }, limit: { amount: 20, in_alarm: false } },
        },
      }),
    );
    renderDialog();

    expect(screen.queryByText("Compute")).not.toBeInTheDocument();
    expect(screen.queryByText("AI Gateway")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Compute/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/AI Gateway/)).not.toBeInTheDocument();
  });
});
