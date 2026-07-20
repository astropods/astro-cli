import { describe, it, expect, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { createHookWrapper } from "@/test/test-utils";
import { deploymentKeys } from "./keys";
import { useChatAgents } from "./chat";
import type {
  AgentDeploymentSummary,
  DeploymentsSummaryResponse,
} from "@/lib/api";

const ACCOUNT = "testuser";

function agent(id: string, name: string): AgentDeploymentSummary {
  return {
    id,
    name,
    build_id: "b1",
    messaging_web_configured: true,
    created_at: "2026-07-10T00:00:00Z",
  };
}

function summary(deploymentIds: string[]): DeploymentsSummaryResponse {
  return {
    accounts: [
      {
        id: "acct-1",
        name: ACCOUNT,
        type: "personal",
        display_name: "Test User",
        deployments: deploymentIds.map((id) => ({
          id,
          name: id,
          status: "active",
        })),
      },
    ],
  };
}

function useSources(summaryIds: string[], listAgents: AgentDeploymentSummary[]) {
  server.use(
    http.get("/api/v1/deployments/summary", () =>
      HttpResponse.json(summary(summaryIds)),
    ),
    http.get("/api/v1/deployments", () =>
      HttpResponse.json({ deployments: listAgents, count: listAgents.length }),
    ),
  );
}

describe("useChatAgents summary reconciliation", () => {
  it("refetches the stale summary when an eligible agent is missing from it", async () => {
    // Summary lags (only agent-a), but the fresher per-account list already has
    // agent-b as chat-eligible. The switcher lists candidates from the summary,
    // so without a refetch agent-b would be missing until a manual refresh.
    useSources(["dep-a"], [agent("dep-a", "agent-a"), agent("dep-b", "agent-b")]);

    const { wrapper, queryClient } = createHookWrapper();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useChatAgents(), { wrapper });

    await waitFor(() =>
      expect(result.current.entries.map((e) => e.deployment.id)).toEqual([
        "dep-a",
        "dep-b",
      ]),
    );

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: deploymentKeys.summary,
      }),
    );
  });

  it("does not refetch the summary when it already lists every eligible agent", async () => {
    useSources(
      ["dep-a", "dep-b"],
      [agent("dep-a", "agent-a"), agent("dep-b", "agent-b")],
    );

    const { wrapper, queryClient } = createHookWrapper();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useChatAgents(), { wrapper });

    await waitFor(() => expect(result.current.entries).toHaveLength(2));
    // Give the reconciliation effect a chance to (wrongly) fire.
    await new Promise((r) => setTimeout(r, 50));

    expect(invalidateSpy).not.toHaveBeenCalledWith({
      queryKey: deploymentKeys.summary,
    });
  });
});
