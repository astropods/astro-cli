import { describe, it, expect } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { AgentDetailContent } from "./AgentDetailContent";
import { afterEach } from "vitest";

afterEach(cleanup);

const baseProps = {
  account: "acme",
  name: "signal-watcher",
  categories: ["MONITORING"],
  safetyPermissions: [],
};

describe("AgentDetailContent", () => {
  it("starts content from second heading when multiple headings exist", () => {
    const readme = [
      "# API Changelog Writer",
      "",
      "Small starter intro block that should be skipped.",
      "",
      "## Quick start",
      "",
      "Run this command.",
    ].join("\n");

    renderWithProviders(
      <AgentDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByRole("heading", { name: /quick start/i })).toBeInTheDocument();
    expect(screen.queryByText(/API Changelog Writer/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/should be skipped/i)).not.toBeInTheDocument();
  });

  it("removes the topmost section even for quick start/prompts patterns", () => {
    const readme = [
      "# Quick start",
      "",
      "Install deps and run setup.",
      "",
      "## Prompts",
      "",
      "Use these prompts in your workflow.",
    ].join("\n");

    renderWithProviders(
      <AgentDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByRole("heading", { name: /prompts/i })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /quick start/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/Install deps and run setup/i)).not.toBeInTheDocument();
  });

  it("keeps content unchanged when only one heading exists", () => {
    const readme = [
      "# Quick start",
      "",
      "Only one section should remain visible.",
    ].join("\n");

    renderWithProviders(
      <AgentDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByRole("heading", { name: /quick start/i })).toBeInTheDocument();
    expect(screen.getByText(/Only one section should remain visible/i)).toBeInTheDocument();
  });

  it("keeps content unchanged when markdown has no headings", () => {
    const readme = "This readme has no markdown headings at all.";

    renderWithProviders(
      <AgentDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByText(readme)).toBeInTheDocument();
  });
});
