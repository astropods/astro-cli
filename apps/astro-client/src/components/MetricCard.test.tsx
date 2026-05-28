import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup } from "@testing-library/react";
import { MetricCard } from "./MetricCard";

afterEach(cleanup);

describe("MetricCard", () => {
  it("renders label and value", () => {
    render(<MetricCard label="Requests today" value="1,284" />);
    expect(screen.getByText("Requests today")).toBeInTheDocument();
    expect(screen.getByText("1,284")).toBeInTheDocument();
  });

  it("shows flat trend by default when no trend value is provided", () => {
    render(<MetricCard label="Tokens" value="84k" />);
    expect(screen.getAllByText("—")).toHaveLength(2);
  });

  it("hides trend indicator when showTrend is false", () => {
    render(<MetricCard label="Tokens" value="84k" showTrend={false} />);
    expect(screen.queryByText("—")).not.toBeInTheDocument();
  });

  it("shows up arrow and green color when trend is positive and higherIsBetter", () => {
    const { container } = render(
      <MetricCard label="Requests" value="100" trend={12} higherIsBetter />
    );
    expect(screen.getByText("12%")).toBeInTheDocument();
    expect(screen.getByText("↑")).toBeInTheDocument();
    expect(container.querySelector(".text-green-700, .dark\\:text-green-400")).toBeTruthy();
  });

  it("shows up arrow and bad color when trend is positive and not higherIsBetter", () => {
    const { container } = render(
      <MetricCard label="Error rate" value="5%" trend={20} higherIsBetter={false} />
    );
    expect(screen.getByText("20%")).toBeInTheDocument();
    expect(screen.getByText("↑")).toBeInTheDocument();
    expect(container.querySelector(".text-coral-600, .dark\\:text-coral-400")).toBeTruthy();
  });

  it("shows down arrow and good color when trend is negative and not higherIsBetter", () => {
    render(<MetricCard label="P95 latency" value="420ms" trend={-8} higherIsBetter={false} />);
    expect(screen.getByText("8%")).toBeInTheDocument();
    expect(screen.getByText("↓")).toBeInTheDocument();
  });

  it("rounds fractional trend percentages", () => {
    render(<MetricCard label="Requests" value="100" trend={12.7} higherIsBetter />);
    expect(screen.getByText("13%")).toBeInTheDocument();
  });

  it("shows value skeleton when loading", () => {
    const { container } = render(<MetricCard label="Requests" value="1,284" loading />);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
    expect(screen.queryByText("1,284")).not.toBeInTheDocument();
  });

  it("shows trend skeleton when trendLoading", () => {
    const { container } = render(
      <MetricCard label="Requests" value="1,284" trendLoading />
    );
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  // The outer tile is rendered through the `<Card>` primitive so it picks
  // up the semantic `bg-card` token (which themes correctly across light
  // and dark) rather than a raw palette utility.
  it("renders the outer container as the <Card> primitive with bg-card", () => {
    const { container } = render(<MetricCard label="x" value="y" />);
    const card = container.querySelector("[data-slot='card']");
    expect(card).not.toBeNull();
    expect(card).toHaveClass("bg-card");
    expect(card).not.toHaveClass("bg-white");
  });

  // Label-style change badge (changePct API, used by activity StatCards).
  // Renders instead of the bold TrendIndicator when `changePct` is set.
  it("shows ↑ and text-success when changePct is positive and showChange", () => {
    const { container } = render(
      <MetricCard label="Cost" value="$10" showChange changePct={25} />
    );
    const badge = container.querySelector(".text-success");
    expect(badge).toBeInTheDocument();
    expect(badge?.textContent).toContain("↑");
  });

  it("shows ↓ and text-destructive when changePct is negative and showChange", () => {
    const { container } = render(
      <MetricCard label="Cost" value="$10" showChange changePct={-15} />
    );
    const badge = container.querySelector(".text-destructive");
    expect(badge).toBeInTheDocument();
    expect(badge?.textContent).toContain("↓");
  });

  it("renders no arrow for changePct=null", () => {
    // Slot is intentionally still in the DOM (height-stable across toggles)
    // but the arrow shouldn't appear when there's no change value.
    render(<MetricCard label="Cost" value="$10" showChange changePct={null} />);
    expect(screen.queryByText("↑")).not.toBeInTheDocument();
    expect(screen.queryByText("↓")).not.toBeInTheDocument();
  });

  it("fades change badge out when showChange=false (keeps slot in DOM for height stability)", () => {
    const { container } = render(
      <MetricCard label="Cost" value="$10" showChange={false} changePct={30} />,
    );
    // The badge wrapper is still rendered so the card height doesn't jump
    // when toggling back and forth between range chips, but it's visually
    // hidden via opacity-0 + aria-hidden.
    const badge = container.querySelector('[aria-hidden="true"]');
    expect(badge).toBeTruthy();
    expect(badge?.className).toContain("opacity-0");
  });

  it("renders sparkline chart when sparkline has more than one point", () => {
    const { container } = render(
      <MetricCard
        label="Cost"
        value="$10"
        sparkline={[1, 2, 3, 4, 5]}
        sparklineDates={["2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04", "2026-01-05"]}
      />
    );
    expect(container.querySelector("[class*='recharts']")).toBeTruthy();
  });

  it("does not render sparkline chart with only one point", () => {
    const { container } = render(
      <MetricCard label="Cost" value="$10" sparkline={[5]} />
    );
    expect(container.querySelector("[class*='recharts']")).toBeFalsy();
  });
});
