import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { AgentCard } from "./AgentCard";

describe("AgentCard", () => {
  it("renders default variant with description", () => {
    renderWithProviders(
      <AgentCard
        slug="acme/signal-watcher"
        account="acme"
        name="signal-watcher"
        description="Monitors API behavior and alert conditions."
      />,
    );

    expect(screen.getByText("signal-watcher")).toBeInTheDocument();
    expect(screen.getByText("Monitors API behavior and alert conditions.")).toBeInTheDocument();
  });

  it("renders often-used-together variant metadata and hides description", () => {
    renderWithProviders(
      <AgentCard
        slug="steve_jobs/alert-router"
        account="steve_jobs"
        name="alert-router"
        description="This text should not render in this variant."
        variant="oftenUsedTogether"
        deployCount={1203}
      />,
    );

    expect(screen.getByText("alert-router")).toBeInTheDocument();
    expect(screen.getByText(/steve_jobs/)).toBeInTheDocument();
    expect(screen.queryByText("4.6")).not.toBeInTheDocument();
    expect(screen.queryByText("1,203")).not.toBeInTheDocument();
    expect(
      screen.queryByText("This text should not render in this variant."),
    ).not.toBeInTheDocument();
  });
});
