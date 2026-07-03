import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { EmptyState } from "./EmptyState";

afterEach(() => {
  cleanup();
});

describe("EmptyState actions", () => {
  it("renders each action as a navigating link", () => {
    render(
      <MemoryRouter>
        <EmptyState
          title="No agents yet"
          actions={[{ label: "Browse", to: "/blueprints" }]}
        />
      </MemoryRouter>,
    );

    const link = screen.getByRole("link", { name: "Browse" });
    expect(link.getAttribute("href")).toBe("/blueprints");
  });
});
