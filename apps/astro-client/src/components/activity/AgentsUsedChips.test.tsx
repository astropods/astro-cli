import { screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { renderWithProviders } from "@/test/test-utils";
import { AgentsUsedChips, type AgentRef } from "./AgentsUsedChips";

afterEach(cleanup);

function ref(name: string, account = "acme"): AgentRef {
  return { name, account };
}

describe("AgentsUsedChips", () => {
  it("renders an em-dash when the list is empty", () => {
    renderWithProviders(<AgentsUsedChips agents={[]} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders one avatar per agent up to maxVisible (default 3) with no overflow indicator", () => {
    const { container } = renderWithProviders(
      <AgentsUsedChips
        agents={[ref("a1"), ref("a2"), ref("a3")]}
      />,
    );
    expect(container.querySelectorAll("img").length).toBe(3);
    expect(container.textContent).not.toMatch(/\+\d/);
  });

  it("renders a +N overflow indicator when agents exceed maxVisible", () => {
    const { container } = renderWithProviders(
      <AgentsUsedChips
        agents={[ref("a1"), ref("a2"), ref("a3"), ref("a4"), ref("a5")]}
      />,
    );
    expect(container.querySelectorAll("img").length).toBe(3);
    expect(container.textContent).toContain("+2");
  });

  it("uses each agent name as the avatar's alt text", () => {
    renderWithProviders(<AgentsUsedChips agents={[ref("alpha"), ref("beta")]} />);
    expect(screen.getByAltText("alpha")).toBeInTheDocument();
    expect(screen.getByAltText("beta")).toBeInTheDocument();
  });

  it("links each avatar under its own publishing account (cross-account / public)", () => {
    renderWithProviders(
      <AgentsUsedChips
        agents={[
          ref("alpha", "acme"),                       // same-account
          ref("research-bot", "anthropic-public"),    // public-blueprint
        ]}
      />,
    );
    const hrefs = screen.getAllByRole("link").map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual(["/acme/alpha", "/anthropic-public/research-bot"]);
  });
});
