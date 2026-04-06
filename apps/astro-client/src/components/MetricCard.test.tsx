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
});
