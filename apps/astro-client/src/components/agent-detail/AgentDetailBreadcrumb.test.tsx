import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { AgentDetailBreadcrumb } from "./AgentDetailBreadcrumb";

afterEach(cleanup);

describe("AgentDetailBreadcrumb", () => {
  it("renders Like and Share action badges", () => {
    renderWithProviders(<AgentDetailBreadcrumb account="acme" agentName="signal-watcher" />);

    expect(screen.getByRole("button", { name: /heart/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /share/i })).toBeInTheDocument();
  });

  it("renders account and agent path in breadcrumb", () => {
    renderWithProviders(<AgentDetailBreadcrumb account="acme" agentName="signal-watcher" />);

    expect(screen.getByText("Browse Agents")).toBeInTheDocument();
    expect(
      screen.getAllByText((_, element) => {
        const text = element?.textContent ?? "";
        return text.includes("acme") && text.includes("signal-watcher");
      }).length,
    ).toBeGreaterThan(0);
  });
});
