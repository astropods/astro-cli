import { describe, it, expect, vi, afterEach } from "vitest";
import { screen, cleanup, fireEvent } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { ActiveContainerAccordion } from "./ActiveContainerAccordion";
import type { ActiveContainerAccordionProps } from "./ActiveContainerAccordion";
import type { K8sEvent } from "@/lib/api";

afterEach(cleanup);

const baseContainer = { name: "app", ready: true, vars: [] };
const containerWithVars = { ...baseContainer, vars: [{ key: "FOO", value: "bar", secret: false, source: "static" }] };
const collectorContainer = { name: "collector", ready: true, vars: [{ key: "FOO", value: "bar", secret: false, source: "static" }] };

const mockEvent: K8sEvent = {
  type: "Normal",
  reason: "Pulled",
  message: "Successfully pulled image",
  object_kind: "Pod",
  object_name: "app-abc",
  count: 1,
  first_timestamp: "2026-04-17T00:00:00Z",
  last_timestamp: "2026-04-17T00:00:01Z",
};

const baseProps: ActiveContainerAccordionProps = {
  workloadName: "agent-workload",
  title: "agent",
  readyText: "1/1",
  uptime: "1d",
  containers: [baseContainer],
  deploymentId: "dep-1",
  deploymentStatus: "active",
  isOpen: true,
  onToggle: vi.fn(),
};

function render(props: Partial<ActiveContainerAccordionProps> = {}) {
  return renderWithProviders(<ActiveContainerAccordion {...baseProps} {...props} />);
}

describe("ActiveContainerAccordion — tab visibility", () => {
  it.each([
    {
      label: "no data → no tabs",
      props: {},
      visible: [],
      hidden: ["Variables", "Domains", "Events"],
    },
    {
      label: "vars only → Variables",
      props: { containers: [containerWithVars] },
      visible: ["Variables"],
      hidden: ["Domains", "Events"],
    },
    {
      label: "urls only → Domains",
      props: { urls: [{ name: "web", url: "https://example.com" }] },
      visible: ["Domains"],
      hidden: ["Variables", "Events"],
    },
    {
      label: "events only → Events",
      props: { events: [mockEvent] },
      visible: ["Events"],
      hidden: ["Variables", "Domains"],
    },
    {
      label: "all data → all tabs",
      props: { containers: [containerWithVars], urls: [{ name: "web", url: "https://example.com" }], events: [mockEvent] },
      visible: ["Variables", "Domains", "Events"],
      hidden: [],
    },
    {
      label: "collector with vars → Variables hidden",
      props: { containers: [collectorContainer] },
      visible: [],
      hidden: ["Variables"],
    },
  ])("$label", ({ props, visible, hidden }) => {
    render(props);
    for (const tab of visible) expect(screen.getByRole("button", { name: tab })).toBeInTheDocument();
    for (const tab of hidden) expect(screen.queryByRole("button", { name: tab })).not.toBeInTheDocument();
  });
});

describe("ActiveContainerAccordion — tab switching", () => {
  it("clicking a tab switches the active view", () => {
    render({ containers: [containerWithVars], events: [mockEvent] });
    fireEvent.click(screen.getByRole("button", { name: "Events" }));
    expect(screen.getByText("Successfully pulled image")).toBeInTheDocument();
  });

  it("defaults to the first available tab when vars are absent", () => {
    render({ events: [mockEvent] });
    expect(screen.getByText("Successfully pulled image")).toBeInTheDocument();
  });
});
