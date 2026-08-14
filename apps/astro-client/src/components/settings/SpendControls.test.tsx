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

// The saved value is the provider's, not the text that was typed. toCents rounds,
// so "50.999" stores 5100 and the field has to read 51: a form still showing
// "50.999" disagrees with the threshold that actually fires.
describe("SpendControls after a save", () => {
  it("shows the stored amount rather than the typed text", async () => {
    mockMutate.mockImplementation((_input, opts) => opts?.onSuccess?.());
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 5000, in_alarm: false } }) },
      isLoading: false,
    });
    const { rerender } = render(<SpendControls account="acme" />);

    const user = userEvent.setup();
    const field = screen.getByLabelText("Stop agents at");
    await user.clear(field);
    await user.type(field, "50.999");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(mockMutate).toHaveBeenCalledWith(
      { warning: null, limit: 5100 },
      expect.anything(),
    );

    // What the query reports once the write has seeded the cache.
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 5100, in_alarm: false } }) },
      isLoading: false,
    });
    rerender(<SpendControls account="acme" />);

    expect(screen.getByLabelText("Stop agents at")).toHaveValue("51");
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
    const field = screen.getByLabelText("Stop agents at");
    await user.clear(field);
    await user.type(field, "500");
    expect(screen.getByLabelText("Stop agents at")).toHaveValue("500");

    // The other account's own limit, as the server reports it.
    mockSpend.mockReturnValue({
      data: { available: true, data: spend({ limit: { amount: 2000, in_alarm: false } }) },
      isLoading: false,
    });
    rerender(<SpendControls account="other-org" />);

    expect(screen.getByLabelText("Stop agents at")).toHaveValue("20");
  });
});
