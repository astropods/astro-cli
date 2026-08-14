import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { BlueprintCard } from "./BlueprintCard";

describe("BlueprintCard", () => {
  it("renders default variant with description", () => {
    renderWithProviders(
      <BlueprintCard
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
      <BlueprintCard
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

  it("shows the owning account in the footer", () => {
    renderWithProviders(
      <BlueprintCard
        slug="acme/signal-watcher"
        account="acme"
        name="signal-watcher"
        description="Monitors API behavior and alert conditions."
        deployCount={4}
      />,
    );

    expect(screen.getByText("acme")).toBeInTheDocument();
    expect(screen.getByText("4 deploys")).toBeInTheDocument();
  });

  it("keeps the title free of the account so the name is never squeezed", () => {
    renderWithProviders(
      <BlueprintCard
        slug="a-very-long-account-name/signal-watcher"
        account="a-very-long-account-name"
        name="signal-watcher"
      />,
    );

    expect(screen.getByRole("heading", { name: "signal-watcher" })).toBeInTheDocument();
  });
});
