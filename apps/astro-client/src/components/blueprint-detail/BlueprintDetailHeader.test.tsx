import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { BlueprintDetailHeader } from "./BlueprintDetailHeader";

afterEach(cleanup);

describe("BlueprintDetailHeader", () => {
  it("shows the account before the blueprint name", () => {
    renderWithProviders(
      <BlueprintDetailHeader account="acme" name="signal-watcher" categories={[]} />,
    );

    // The gap around the slash is flex spacing, not whitespace in the DOM.
    const heading = screen.getByRole("heading", { level: 1 });
    expect(heading.textContent).toBe("acme/signal-watcher");
  });

  it("links the account to its profile", () => {
    renderWithProviders(
      <BlueprintDetailHeader account="acme" name="signal-watcher" categories={[]} />,
    );

    expect(screen.getByRole("link", { name: "acme" })).toHaveAttribute("href", "/acme");
  });

  it("shows the blueprint name in full when the account is long", () => {
    const account = "a".repeat(39);
    renderWithProviders(
      <BlueprintDetailHeader account={account} name="signal-watcher" categories={[]} />,
    );

    // The account may be visually truncated by CSS, but both values stay in the DOM.
    expect(screen.getByText(account)).toBeInTheDocument();
    expect(screen.getByText("signal-watcher")).toBeInTheDocument();
  });

  it("renders categories and the draft badge", () => {
    renderWithProviders(
      <BlueprintDetailHeader
        account="acme"
        name="signal-watcher"
        categories={["PRODUCTIVITY", "SLACK"]}
        isDraft
      />,
    );

    expect(screen.getByText("PRODUCTIVITY")).toBeInTheDocument();
    expect(screen.getByText("SLACK")).toBeInTheDocument();
    expect(screen.getByText("Finish setup")).toBeInTheDocument();
  });
});
