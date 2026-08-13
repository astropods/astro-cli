import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import type { AgentDeployment, DeploymentStatus } from "@/lib/api";
import { ApiRequestError } from "@/lib/api";
import { AgentStatusToggle } from "./AgentStatusToggle";

// The live status the toggle reads; set per test before rendering.
let mockStatus: DeploymentStatus | undefined;
// The wakeup mutation's behaviour, so a test can make the server refuse.
let mockWakeup = vi.fn();
const mockToastError = vi.fn();

vi.mock("sonner", () => ({ toast: { error: (m: string) => mockToastError(m) } }));

vi.mock("@/api/queries/deployments", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/deployments")>();
  return {
    ...actual,
    useDeploymentStatus: (() => ({ data: mockStatus })) as unknown as typeof actual.useDeploymentStatus,
    useStopDeployment: (() => ({ mutate: vi.fn(), isPending: false })) as unknown as typeof actual.useStopDeployment,
    useWakeUpDeployment: (() => ({ mutate: mockWakeup, isPending: false })) as unknown as typeof actual.useWakeUpDeployment,
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
  mockWakeup = vi.fn();
  mockToastError.mockReset();
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

// The mutations had no onError, so a refusal cleared nothing and said nothing:
// the switch settled back with no explanation. Now that the operate routes gate
// on billing, that silence is the first thing a gated account meets.
describe("AgentStatusToggle when the server refuses the wakeup", () => {
  const stopped: AgentDeployment = { ...base, status: "Stopped" };

  function refuseWith(body: Record<string, unknown>) {
    mockWakeup = vi.fn((_vars, opts?: { onError?: (e: unknown) => void }) => {
      opts?.onError?.(new ApiRequestError(body, 402));
    });
  }

  // The sentence is the server's `details`, not copy assembled here from a
  // reason, so the toast cannot contradict the 402 or the app-shell banner.
  it("shows the server's own explanation", async () => {
    refuseWith({
      error: "Billing suspended",
      code: "BILLING_SUSPENDED",
      details: "This account reached its spend limit. Contact support to raise it.",
    });
    renderToggle(stopped);

    await userEvent.click(screen.getByRole("switch"));
    expect(mockToastError).toHaveBeenCalledWith(
      expect.stringContaining("Contact support to raise it"),
    );
  });

  // A body with no prose still has to reach the owner. ApiRequestError builds
  // "Request failed with status 402", which getApiErrorMessage prefers over the
  // caller's fallback, so the toast fires either way.
  it("still speaks when the body carries no explanation", async () => {
    refuseWith({});
    renderToggle(stopped);

    await userEvent.click(screen.getByRole("switch"));
    expect(mockToastError).toHaveBeenCalledWith(expect.stringContaining("402"));
  });
});
