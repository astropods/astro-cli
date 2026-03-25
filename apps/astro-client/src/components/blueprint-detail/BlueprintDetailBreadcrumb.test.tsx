import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { BlueprintDetailBreadcrumb } from "./BlueprintDetailBreadcrumb";

afterEach(cleanup);

describe("BlueprintDetailBreadcrumb", () => {
  it("renders Like and Share action badges", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getByRole("button", { name: /heart/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /share/i })).toBeInTheDocument();
  });

  it("renders account and agent path in breadcrumb", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getByText("Blueprints")).toBeInTheDocument();
    expect(
      screen.getAllByText((_, element) => {
        const text = element?.textContent ?? "";
        return text.includes("acme") && text.includes("signal-watcher");
      }).length,
    ).toBeGreaterThan(0);
  });
});
