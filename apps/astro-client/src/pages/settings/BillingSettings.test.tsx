import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import BillingSettings from "./BillingSettings";

const mockUseAuth = vi.fn();

vi.mock("@/lib/auth", () => ({ useAuth: () => mockUseAuth() }));
vi.mock("@/components/settings/BillingView", () => ({
  BillingView: ({ account }: { account: string }) => <div>BillingView:{account}</div>,
}));

describe("BillingSettings while auth is still resolving", () => {
  it("renders nothing rather than passing an empty account that reads as billing-disabled", () => {
    mockUseAuth.mockReturnValue({ personalAccount: undefined, isLoading: true });
    const { container } = render(<BillingSettings />);

    expect(container).toBeEmptyDOMElement();
  });
});

describe("BillingSettings once auth has resolved", () => {
  it("renders BillingView with the personal account's name", () => {
    mockUseAuth.mockReturnValue({ personalAccount: { name: "acme" }, isLoading: false });
    render(<BillingSettings />);

    expect(screen.getByText("BillingView:acme")).toBeInTheDocument();
  });
});
