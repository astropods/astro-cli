import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { BillingSpend } from "@/lib/api";
import { PlanSummary } from "./PlanSummary";

const mockSpend = vi.fn();

vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: () => mockSpend(),
}));

beforeEach(() => {
  mockSpend.mockReset();
});

function spend(partial: Partial<BillingSpend> = {}): BillingSpend {
  return {
    currency: "USD (cents)",
    current_spend: 0,
    has_current_spend: false,
    usage_spend: 0,
    has_usage_spend: false,
    credit_remaining: 0,
    has_credit: false,
    ...partial,
  };
}

function renderPlan(data: BillingSpend) {
  mockSpend.mockReturnValue({ data: { available: true, data }, isLoading: false });
  return render(<PlanSummary account="acme" />);
}

describe("PlanSummary", () => {
  it.each([
    ["unlimited", "Unlimited"],
    ["credit", "Signup credit"],
    ["no_credit", "Pay as you go"],
  ])("names the %s plan", (plan, label) => {
    renderPlan(spend({ plan }));
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("says so when no contract covers the account", () => {
    renderPlan(spend());
    expect(screen.getByText("Not set up")).toBeInTheDocument();
  });

  it("treats an unknown plan as not set up", () => {
    renderPlan(spend({ plan: "enterprise" }));
    expect(screen.getByText("Not set up")).toBeInTheDocument();
  });

  it("shows the remaining credit only on the credit plan", () => {
    renderPlan(spend({ plan: "credit", credit_remaining: 7.5, has_credit: true }));
    expect(screen.getByText("$7.50 credit left")).toBeInTheDocument();
  });

  it("does not show a credit balance on unlimited", () => {
    renderPlan(spend({ plan: "unlimited", credit_remaining: 7.5, has_credit: true }));
    expect(screen.queryByText(/credit left/)).not.toBeInTheDocument();
  });
});
