import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { AgentDetailContent } from "./AgentDetailContent";

const baseProps = {
  account: "acme",
  name: "signal-watcher",
  categories: ["MONITORING"],
  safetyPermissions: [],
};

describe("AgentDetailContent", () => {
  it("starts resume content from the second heading when available", () => {
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
});
