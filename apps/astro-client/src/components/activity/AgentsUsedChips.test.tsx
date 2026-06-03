import { screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderWithProviders } from "@/test/test-utils";
import { AgentsUsedChips, type AgentRef } from "./AgentsUsedChips";
import type { AgentDeploymentRef } from "./AgentNameLink";

let depCounter = 0;
afterEach(() => {
  cleanup();
  // Reset the module-level counter so test ordering can't bleed
  // deployment-id values between cases.
  depCounter = 0;
});

function ref(name: string, account = "acme", deploymentID?: string): AgentRef {
  return { deployment_id: deploymentID ?? `dep-${++depCounter}`, name, account };
}

function depIndex(entries: Array<{ name: string; deps: AgentDeploymentRef[] }>): Map<string, AgentDeploymentRef[]> {
  return new Map(entries.map((e) => [e.name, e.deps]));
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

  // When deploymentsByAgent is undefined (initial render / still loading),
  // chips must NOT flash "(deleted)" — that state is reserved for the
  // index-loaded-but-deployment-missing case (genuinely tombstoned). The
  // assertion that matters here is the absence of "(deleted)" anywhere
  // in the rendered output (sr-only, tooltip, popover) — proving the
  // muted/deleted treatment isn't applied during the load window.
  it("does not mark chips as deleted while deploymentsByAgent is undefined", async () => {
    renderWithProviders(
      <AgentsUsedChips agents={[ref("weather-poet", "acme", "dep-1")]} />,
    );
    await userEvent.hover(screen.getByRole("link"));
    expect(screen.queryByText(/\(deleted\)/)).toBeNull();
  });

  // Without deploymentsByAgent (or when a deployment is absent from the live
  // index), chips fall back to the blueprint detail page — covers the
  // archived-only-spend / cross-account-public-deploy paths.
  it("falls back to the blueprint detail route when the deployment is unknown", () => {
    renderWithProviders(
      <AgentsUsedChips
        agents={[
          ref("alpha", "acme", "dep-1"),
          ref("research-bot", "anthropic-public", "dep-2"),
        ]}
      />,
    );
    const hrefs = screen.getAllByRole("link").map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual(["/acme/alpha", "/anthropic-public/research-bot"]);
  });

  // With a deploymentsByAgent index, each chip routes to its specific
  // deployment's Monitor tab — this is what differentiates two chips of
  // the same blueprint (same avatar, different click target + tooltip).
  it("routes to per-deployment monitor when deployments are indexed", async () => {
    renderWithProviders(
      <AgentsUsedChips
        agents={[
          ref("weather-poet", "acme", "dep-east"),
          ref("weather-poet", "acme", "dep-west"),
        ]}
        deploymentsByAgent={depIndex([
          {
            name: "weather-poet",
            deps: [
              { id: "dep-east", name: "weather-poet", display_name: "Weather (east)", namespace: "us-east-1" },
              { id: "dep-west", name: "weather-poet", display_name: "Weather (west)", namespace: "us-west-2" },
            ],
          },
        ])}
      />,
    );
    const links = screen.getAllByRole("link");
    expect(links.map((a) => a.getAttribute("href"))).toEqual([
      "/acme/agents/dep-east/monitor",
      "/acme/agents/dep-west/monitor",
    ]);

    // Hover the first chip → tooltip shows display_name. namespace is
    // intentionally NOT appended (it's a K8s-generated handle, not
    // user-meaningful). display_name is the per-deployment differentiator
    // between two chips of the same blueprint. The label also appears in
    // the sr-only summary span at the top of the chip row, so
    // findAllByText returns both copies — assert at least one is inside
    // the live tooltip popover to confirm the hover path actually fired.
    await userEvent.hover(links[0]);
    const matches = await screen.findAllByText("Weather (east)");
    expect(matches.length).toBeGreaterThan(0);
    expect(matches.some((el) => el.closest('[data-slot="tooltip-content"]') !== null)).toBe(true);
  });
});
