import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { BillingStatusResponse } from "@/lib/api";
import { BillingStatusBanner } from "./BillingStatusBanner";

const mockStatus = vi.fn();
const mockNavigate = vi.fn();
vi.mock("@/api/queries/billing", () => ({
  useBillingStatus: (account: string) => mockStatus(account),
}));
vi.mock("@/hooks/use-active-account", () => ({
  useActiveAccount: () => ({ activeAccount: "acme", setActiveAccount: vi.fn() }),
}));
vi.mock("react-router", () => ({
  useNavigate: () => mockNavigate,
}));

beforeEach(() => {
  mockStatus.mockReset();
  mockNavigate.mockReset();
});

function status(partial: Partial<BillingStatusResponse>): BillingStatusResponse {
  return {
    status: "active",
    credits_exhausted: false,
    has_payment_method: false,
    enforced: true,
    workloads_suspended: false,
    ...partial,
  };
}

describe("BillingStatusBanner", () => {
  it("renders nothing for an active account", () => {
    mockStatus.mockReturnValue({ data: status({}) });
    const { container } = render(<BillingStatusBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing while the status is still loading", () => {
    mockStatus.mockReturnValue({ data: undefined });
    const { container } = render(<BillingStatusBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("prompts for a card when free credits run out", () => {
    mockStatus.mockReturnValue({
      data: status({
        status: "suspended",
        reason: "credits_exhausted",
        credits_exhausted: true,
      }),
    });
    render(<BillingStatusBanner />);
    expect(screen.getByText("Free credits used up")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add payment method" })).toBeInTheDocument();
  });

  // The pay-as-you-go case: credits are spent but a card covers the overage, so
  // the account is active and must not be nagged.
  it("stays silent once a card makes the account pay-as-you-go", () => {
    mockStatus.mockReturnValue({
      data: status({ credits_exhausted: true, has_payment_method: true }),
    });
    const { container } = render(<BillingStatusBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("softens the copy for past_due, which has not stopped agents yet", () => {
    mockStatus.mockReturnValue({
      data: status({ status: "past_due", reason: "dunning", has_payment_method: true }),
    });
    render(<BillingStatusBanner />);
    expect(screen.getByText("Payment failed")).toBeInTheDocument();
    expect(screen.getByText(/agents are stopped once the grace period ends/)).toBeInTheDocument();
  });

  it("stays silent in observe mode, where nothing is actually suspended", () => {
    mockStatus.mockReturnValue({
      data: status({ status: "suspended", reason: "credits_exhausted", enforced: false }),
    });
    const { container } = render(<BillingStatusBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("keeps warning while workloads are still stopped after enforcement is off", () => {
    mockStatus.mockReturnValue({
      data: status({
        status: "suspended",
        reason: "credits_exhausted",
        enforced: false,
        workloads_suspended: true,
      }),
    });
    render(<BillingStatusBanner />);
    expect(screen.getByText(/free credits/i)).toBeInTheDocument();
  });

  // A write-off outranks exhaustion server-side, and adding a card does not
  // lift it, so the credit copy must not win just because the latch is set.
  it("does not offer a card when a higher-priority reason stopped the account", () => {
    mockStatus.mockReturnValue({
      data: status({
        status: "suspended",
        reason: "uncollectible",
        credits_exhausted: true,
        has_payment_method: false,
      }),
    });
    render(<BillingStatusBanner />);
    expect(screen.getByText(/could not be collected/i)).toBeInTheDocument();
    expect(screen.queryByText(/free credits/i)).not.toBeInTheDocument();
  });

  it("still infers the credit copy when the server sent no reason", () => {
    mockStatus.mockReturnValue({
      data: status({ status: "suspended", credits_exhausted: true }),
    });
    render(<BillingStatusBanner />);
    expect(screen.getByText(/free credits/i)).toBeInTheDocument();
  });

  it("falls back to generic copy for a reason this build does not know", () => {
    mockStatus.mockReturnValue({
      data: status({ status: "suspended", reason: "some_future_reason" }),
    });
    render(<BillingStatusBanner />);
    expect(screen.getByText("Billing issue — agents stopped")).toBeInTheDocument();
  });
});
