import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AgentDeployment, DeploymentStatus } from "@/lib/api";
import { AgentStatusToggle } from "./AgentStatusToggle";

// The live status the toggle reads; set per test before rendering.
let mockStatus: DeploymentStatus | undefined;

vi.mock("@/api/queries/deployments", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/deployments")>();
  return {
    ...actual,
    useDeploymentStatus: (() => ({ data: mockStatus })) as unknown as typeof actual.useDeploymentStatus,
    useStopDeployment: (() => ({ mutate: vi.fn(), isPending: false })) as unknown as typeof actual.useStopDeployment,
    useWakeUpDeployment: (() => ({ mutate: vi.fn(), isPending: false })) as unknown as typeof actual.useWakeUpDeployment,
  };
});

const base: AgentDeployment = {
  id: "dep-1",
  name: "test-agent",
  display_name: "Test Agent",
  build_id: "b1",
  namespace: "ns",
  status: "Running",
  replicas: 1,
  created_at: "2026-01-01T00:00:00Z",
  components: [],
};

function renderToggle(deployment: AgentDeployment) {
  const qc = new QueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <AgentStatusToggle deployment={deployment} account="acme" />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  mockStatus = undefined;
});

describe("AgentStatusToggle", () => {
  it("shows Active when the live status is active", () => {
    mockStatus = { value: "active", reason: "ready", details: "" };
    renderToggle(base);
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("shows Error (never Active) for a failed deploy whose record status is not stopped", () => {
    // On failure the record status stays non-stopped, so `checked` reads true;
    // the live "error" value must win so a broken agent never renders as Active.
    mockStatus = { value: "error", reason: "failed", details: "Deployment failed" };
    renderToggle({ ...base, status: "error" });
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.queryByText("Active")).not.toBeInTheDocument();
  });

  it("shows Paused when the record status is stopped", () => {
    mockStatus = { value: "inactive", reason: "paused", details: "" };
    renderToggle({ ...base, status: "Stopped" });
    expect(screen.getByText("Paused")).toBeInTheDocument();
  });
});
