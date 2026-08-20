import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { BillingSpend } from "@/lib/api";
import { SpendControls } from "./SpendControls";

const mockSpend = vi.fn();
const mockMutate = vi.fn();
const mockUsageMutate = vi.fn();
const mockToastError = vi.fn();

vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: () => mockSpend(),
  useSetBillingSpendThresholds: () => ({ mutateAsync: mockMutate, isPending: false }),
  useSetBillingUsageThresholds: () => ({ mutateAsync: mockUsageMutate, isPending: false }),
}));
vi.mock("sonner", () => ({
  toast: { error: (m: string) => mockToastError(m), success: vi.fn() },
}));

beforeEach(() => {
  mockSpend.mockReset();
  mockMutate.mockReset();
  mockMutate.mockResolvedValue(undefined);
  mockUsageMutate.mockReset();
  mockUsageMutate.mockResolvedValue(undefined);
  mockToastError.mockReset();
});

function spend(partial: Partial<BillingSpend> = {}): BillingSpend {
  return {
    currency: "USD (cents)",
    // Spend arrives converted; thresholds arrive as the provider's cents.
    current_spend: 12.34,
    has_current_spend: true,
    usage_spend: 12.34,
    has_usage_spend: true,
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
    expect(screen.getByLabelText("Spend: stop agents at")).toHaveValue("50");
    expect(screen.getByText(/\$12\.34 used this period/)).toBeInTheDocument();
  });

  it("sends dollars back as cents", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Spend: stop agents at"), "75");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).toHaveBeenCalledWith({ warning: null, limit: 7500 });
  });

  // An empty field clears the threshold. Sending zero instead would cap the
  // account at nothing and stop every agent it has.
  it("clears a threshold rather than setting it to zero", async () => {
    renderControls(spend({ limit: { amount: 5000, in_alarm: false } }));
    const user = userEvent.setup();
    await user.clear(screen.getByLabelText("Spend: stop agents at"));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).toHaveBeenCalledWith({ warning: null, limit: null });
  });

  // The limit suspends the account first, so a warning at or above it never
  // fires. The server rejects this too; catching it here explains why.
  it("refuses a warning that the limit would pre-empt", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Spend: warn me at"), "80");
    await user.type(screen.getByLabelText("Spend: stop agents at"), "50");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalledWith(expect.stringContaining("below the limit"));
  });

  it("refuses a negative amount", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Spend: stop agents at"), "-5");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("offers Save only once a field has changed", async () => {
    renderControls(spend());
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.setup().type(screen.getByLabelText("Compute: stop agents at"), "5");
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
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

// The saved value is the provider's, not the text that was typed. toCents rounds,
// so "50.999" stores 5100 and the field has to read 51: a form still showing
// "50.999" disagrees with the threshold that actually fires.
describe("SpendControls after a save", () => {
  it("shows the stored amount rather than the typed text", async () => {
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 5000, in_alarm: false } }) },
      isLoading: false,
    });
    const { rerender } = render(<SpendControls account="acme" />);

    const user = userEvent.setup();
    const field = screen.getByLabelText("Spend: stop agents at");
    await user.clear(field);
    await user.type(field, "50.999");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(mockMutate).toHaveBeenCalledWith({ warning: null, limit: 5100 });

    // What the query reports once the write has seeded the cache.
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 5100, in_alarm: false } }) },
      isLoading: false,
    });
    rerender(<SpendControls account="acme" />);

    expect(screen.getByLabelText("Spend: stop agents at")).toHaveValue("51");
  });
});

// Switching accounts changes a prop, not a route component: OrgBillingSettings
// reads orgSlug from useParams, so navigating between two orgs re-renders rather
// than remounts. An edited field that survives that carries one account's number
// onto another's form, and the next Save writes it there.
describe("SpendControls across an account switch", () => {
  it("does not carry an edited value onto the next account", async () => {
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 5000, in_alarm: false } }) },
      isLoading: false,
    });
    const { rerender } = render(<SpendControls account="acme" />);

    const user = userEvent.setup();
    const field = screen.getByLabelText("Spend: stop agents at");
    await user.clear(field);
    await user.type(field, "500");
    expect(screen.getByLabelText("Spend: stop agents at")).toHaveValue("500");

    // The other account's own limit, as the server reports it.
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 2000, in_alarm: false } }) },
      isLoading: false,
    });
    rerender(<SpendControls account="other-org" />);

    expect(screen.getByLabelText("Spend: stop agents at")).toHaveValue("20");
  });

  // The provider measures a threshold against usage before credit drawdown, so
  // showing the invoice total prints $0.00 for any account on signup credit,
  // right up until its own warning fires.
  it("shows usage before credit, not the credit-offset bill", () => {
    renderControls(
      spend({ current_spend: 0, has_current_spend: true, usage_spend: 2.76, has_usage_spend: true }),
    );
    expect(screen.getByText(/\$2\.76 used this period/)).toBeInTheDocument();
  });

  // Spend and thresholds share one response in different units. Dividing spend
  // by 100 alongside the thresholds renders a real bill as a hundredth of itself.
  it("renders spend and a threshold at the same scale", () => {
    renderControls(
      spend({ usage_spend: 50, has_usage_spend: true, limit: { amount: 5000, in_alarm: false } }),
    );
    expect(screen.getByText(/\$50\.00 used this period/)).toBeInTheDocument();
    expect(screen.getByLabelText("Spend: stop agents at")).toHaveValue("50");
  });
});

describe("SpendControls usage rows", () => {
  it("sends a usage cap as typed, with no currency conversion", async () => {
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Compute: stop agents at"), "500");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockUsageMutate).toHaveBeenCalledWith({ metric: "compute", warning: null, limit: 500 });
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("writes only the rows the owner edited", async () => {
    renderControls(spend({ usage: { compute: { unit: "CU-hours", limit: { amount: 40, in_alarm: false } } } }));
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("AI Gateway: warn me at"), "25");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(mockUsageMutate).toHaveBeenCalledTimes(1);
    expect(mockUsageMutate).toHaveBeenCalledWith({ metric: "gateway", warning: 25, limit: null });
  });

  it("hides the spend row on a plan that cannot fire it", () => {
    renderControls(spend({ plan: "unlimited" }));
    expect(screen.queryByLabelText("Spend: stop agents at")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Compute: stop agents at")).toBeInTheDocument();
  });

  it("keeps the edits for a row that failed and clears the ones that landed", async () => {
    mockUsageMutate.mockImplementation(({ metric }: { metric: string }) =>
      metric === "gateway" ? Promise.reject(new Error("provider unavailable")) : Promise.resolve(),
    );
    renderControls(spend());
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Compute: stop agents at"), "500");
    await user.type(screen.getByLabelText("AI Gateway: stop agents at"), "25");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(mockToastError).toHaveBeenCalledTimes(1));
    expect(mockToastError).toHaveBeenCalledWith(expect.stringContaining("AI Gateway"));
    // Both fields fall back to what the server holds, which the stub reports as
    // no cap on either metric.
    expect(screen.getByLabelText("AI Gateway: stop agents at")).toHaveValue("");
    expect(screen.getByLabelText("Compute: stop agents at")).toHaveValue("");
  });

  it("shows a usage cap the provider reports crossed", () => {
    renderControls(spend({ usage: { gateway: { unit: "USD of model usage", limit: { amount: 10, in_alarm: true } } } }));
    expect(screen.getByLabelText("AI Gateway: stop agents at")).toHaveValue("10");
    expect(screen.getByText("Reached")).toBeInTheDocument();
  });
});
