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
};

describe("AgentDetailContent", () => {
  it("renders full markdown content as provided", () => {
    const readme = [
      "# API Changelog Writer",
      "",
      "Small starter intro block that should be shown.",
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

    expect(screen.getByRole("heading", { name: /api changelog writer/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /quick start/i })).toBeInTheDocument();
    expect(screen.getByText(/should be shown/i)).toBeInTheDocument();
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
