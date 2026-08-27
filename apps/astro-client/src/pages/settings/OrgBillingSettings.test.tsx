import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import OrgBillingSettings from "./OrgBillingSettings";

const mockUseAuth = vi.fn();
const mockUseParams = vi.fn();

vi.mock("react-router", () => ({
  useParams: () => mockUseParams(),
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => <a href={to}>{children}</a>,
}));
vi.mock("@/lib/auth", () => ({ useAuth: () => mockUseAuth() }));
vi.mock("@/components/settings/BillingView", () => ({
  BillingView: ({ account }: { account: string }) => <div>BillingView:{account}</div>,
}));

describe("OrgBillingSettings", () => {
  it("renders BillingView for an org admin", () => {
    mockUseParams.mockReturnValue({ orgSlug: "acme" });
    mockUseAuth.mockReturnValue({ role: "admin" });

    render(<OrgBillingSettings />);

    expect(screen.getByText("BillingView:acme")).toBeInTheDocument();
  });

  it("renders BillingView for an org owner", () => {
    mockUseParams.mockReturnValue({ orgSlug: "acme" });
    mockUseAuth.mockReturnValue({ role: "owner" });

    render(<OrgBillingSettings />);

    expect(screen.getByText("BillingView:acme")).toBeInTheDocument();
  });

  it("blocks a member from viewing org billing by direct navigation", () => {
    mockUseParams.mockReturnValue({ orgSlug: "acme" });
    mockUseAuth.mockReturnValue({ role: "member" });

    render(<OrgBillingSettings />);

    expect(screen.getByText("Access denied")).toBeInTheDocument();
    expect(screen.queryByText("BillingView:acme")).not.toBeInTheDocument();
  });

  it("blocks a session with no role at all, not just a named non-admin role", () => {
    mockUseParams.mockReturnValue({ orgSlug: "acme" });
    mockUseAuth.mockReturnValue({ role: null });

    render(<OrgBillingSettings />);

    expect(screen.getByText("Access denied")).toBeInTheDocument();
  });

  it("sends a denied member back to their own org's general settings", () => {
    mockUseParams.mockReturnValue({ orgSlug: "acme" });
    mockUseAuth.mockReturnValue({ role: "member" });

    render(<OrgBillingSettings />);

    expect(screen.getByRole("link", { name: "Back to organization settings" })).toHaveAttribute(
      "href",
      "/settings/org/acme/general",
    );
  });
});
