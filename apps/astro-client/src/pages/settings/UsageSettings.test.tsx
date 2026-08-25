import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import UsageSettings from "./UsageSettings";

const mockUseAuth = vi.fn();

vi.mock("@/lib/auth", () => ({ useAuth: () => mockUseAuth() }));
vi.mock("@/components/settings/UsageView", () => ({
  UsageView: ({ account }: { account: string }) => <div>UsageView:{account}</div>,
}));

describe("UsageSettings while auth is still resolving", () => {
  it("renders nothing rather than passing an empty account that reads as billing-disabled", () => {
    mockUseAuth.mockReturnValue({ personalAccount: undefined, isLoading: true });
    const { container } = render(<UsageSettings />);

    expect(container).toBeEmptyDOMElement();
  });
});

describe("UsageSettings once auth has resolved", () => {
  it("renders UsageView with the personal account's name", () => {
    mockUseAuth.mockReturnValue({ personalAccount: { name: "acme" }, isLoading: false });
    render(<UsageSettings />);

    expect(screen.getByText("UsageView:acme")).toBeInTheDocument();
  });
});
