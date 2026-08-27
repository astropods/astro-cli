import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ClusterPicker } from "./ClusterPicker";

const mockUseAccount = vi.fn();

vi.mock("@/api/queries/accounts", () => ({
  useAccount: (...args: unknown[]) => mockUseAccount(...args),
}));

const allowedClusters = [
  {
    cluster_id: "primary-us",
    region: "us-east-1",
    region_label: "US East (N. Virginia)",
    region_flag: "🇺🇸",
    is_default: true,
  },
  {
    cluster_id: "managed-eu",
    region: "eu-west-1",
    region_label: "Europe (Ireland)",
    region_flag: "🇮🇪",
  },
];

afterEach(cleanup);

beforeEach(() => {
  mockUseAccount.mockReturnValue({ data: { allowed_clusters: allowedClusters } });
});

describe("ClusterPicker", () => {
  it("renders compact native radio choices without flag emoji", () => {
    render(
      <ClusterPicker
        account="astro"
        value=""
        currentClusterId="primary-us"
        onChange={() => {}}
      />,
    );

    expect(screen.getByText("Select where this agent runs.")).toBeInTheDocument();
    const group = screen.getByRole("radiogroup", { name: "Deployment region" });
    const us = within(group).getByRole("radio", { name: /US East \(N\. Virginia\).*Default.*us-east-1/ });
    const eu = within(group).getByRole("radio", { name: /Europe \(Ireland\).*eu-west-1/ });

    expect(us).toBeChecked();
    expect(eu).not.toBeChecked();
    expect(screen.queryByText("🇺🇸")).not.toBeInTheDocument();
    expect(screen.queryByText("🇮🇪")).not.toBeInTheDocument();
  });

  it("reports the selected cluster id", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ClusterPicker
        account="astro"
        value="primary-us"
        currentClusterId="primary-us"
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("radio", { name: /Europe \(Ireland\)/ }));
    expect(onChange).toHaveBeenCalledWith("managed-eu");
  });

  it("puts migration guidance before the choices when changing a deployed agent", () => {
    render(
      <ClusterPicker
        account="astro"
        value="managed-eu"
        currentClusterId="primary-us"
        deployed
        onChange={() => {}}
      />,
    );

    const guidance = screen.getByRole("status");
    const group = screen.getByRole("radiogroup", { name: "Deployment region" });
    expect(guidance).toHaveTextContent(
      "Deploying moves this agent to the new region. It restarts, and its current region is torn down.",
    );
    expect(guidance.compareDocumentPosition(group) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("shows a single allowed region as a selected disabled value with an explanation", async () => {
    const user = userEvent.setup();
    mockUseAccount.mockReturnValue({ data: { allowed_clusters: [allowedClusters[0]] } });
    render(
      <ClusterPicker
        account="astro"
        value=""
        currentClusterId="primary-us"
        onChange={() => {}}
      />,
    );

    expect(screen.getByText("This agent runs in the only available region.")).toBeInTheDocument();
    const region = screen.getByRole("radio", { name: /US East \(N\. Virginia\).*Default.*us-east-1/ });
    expect(region).toBeChecked();
    expect(region).toBeDisabled();

    await user.tab();
    expect(region.closest("label")).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Region availability is set for this account. Contact support to request another region.",
    );
  });
});
