import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { BlueprintDetailBreadcrumb } from "./BlueprintDetailBreadcrumb";

afterEach(cleanup);

describe("BlueprintDetailBreadcrumb", () => {
  it("renders Like and Share action badges", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getAllByRole("button", { name: /heart/i })).not.toHaveLength(0);
    expect(screen.getAllByRole("button", { name: /share/i })).not.toHaveLength(0);
  });

  it("renders account and agent path in breadcrumb", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getByText("Blueprints")).toBeInTheDocument();
    expect(screen.getByText("signal-watcher")).toBeInTheDocument();

    const accountLinks = screen.getAllByRole("link", { name: /acme/i });
    expect(accountLinks.length).toBeGreaterThan(0);
    for (const link of accountLinks) {
      expect(link).toHaveAttribute("href", "/acme");
    }
  });
});
