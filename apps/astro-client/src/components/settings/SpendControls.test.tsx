import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { BillingSpend } from "@/lib/api";
import { SpendControls } from "./SpendControls";

const mockSpend = vi.fn();
const mockMutate = vi.fn();
const mockToastError = vi.fn();

vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: () => mockSpend(),
  useSetBillingSpendThresholds: () => ({ mutate: mockMutate, isPending: false }),
}));
vi.mock("sonner", () => ({
  toast: { error: (m: string) => mockToastError(m), success: vi.fn() },
}));

beforeEach(() => {
  mockSpend.mockReset();
  mockMutate.mockReset();
  mockToastError.mockReset();
});

function spend(partial: Partial<BillingSpend> = {}): BillingSpend {
  return {
    currency: "USD (cents)",
    current_spend: 1234,
    has_current_spend: true,
    credit_remaining: 0,
    has_credit: false,
    ...partial,
  };
}

function renderControls(data: BillingSpend) {
  mockSpend.mockReturnValue({ data: { available: true, data }, isLoading: false });
  return render(<SpendControls account="acme" />);
}

describe("SpendControls", () => {
  // The provider stores cents; a form that showed them would read as 100x the
  // real number and invite a limit set 100x too high.
  it("shows the provider's cents as dollars", () => {
    renderControls(spend({ limit: { amount: 5000, in_alarm: false } }));
    expect(screen.getByLabelText("Stop agents at")).toHaveValue("50");
    expect(screen.getByText(/\$12\.34 this period/)).toBeInTheDocument();
  });

  it("sends dollars back as cents", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Stop agents at"), "75");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).toHaveBeenCalledWith(
      { warning: null, limit: 7500 },
      expect.anything(),
    );
  });

  // An empty field clears the threshold. Sending zero instead would cap the
  // account at nothing and stop every agent it has.
  it("clears a threshold rather than setting it to zero", async () => {
    renderControls(spend({ limit: { amount: 5000, in_alarm: false } }));
    const user = userEvent.setup();
    await user.clear(screen.getByLabelText("Stop agents at"));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).toHaveBeenCalledWith(
      { warning: null, limit: null },
      expect.anything(),
    );
  });

  // The limit suspends the account first, so a warning at or above it never
  // fires. The server rejects this too; catching it here explains why.
  it("refuses a warning that the limit would pre-empt", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Warn me at"), "80");
    await user.type(screen.getByLabelText("Stop agents at"), "50");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalledWith(expect.stringContaining("below the limit"));
  });

  it("refuses a negative amount", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Stop agents at"), "-5");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).not.toHaveBeenCalled();
  });

  // in_alarm comes from the provider's own evaluation, and it is the only thing
  // telling an owner they are already over.
  it("marks a threshold the provider reports crossed", () => {
    renderControls(spend({ limit: { amount: 5000, in_alarm: true } }));
    expect(screen.getByText("Reached")).toBeInTheDocument();
  });

  // A backend with no spend controls reports unavailable, and a form for
  // settings that cannot be saved is worse than none.
  it("renders nothing when the backend has no billing", () => {
    mockSpend.mockReturnValue({ data: { available: false }, isLoading: false });
    const { container } = render(<SpendControls account="acme" />);
    expect(container).toBeEmptyDOMElement();
  });
});
