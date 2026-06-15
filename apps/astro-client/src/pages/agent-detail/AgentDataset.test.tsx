import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import type { EvalDatasetResponse } from "@/lib/api";
import AgentDataset from "./AgentDataset";

afterEach(cleanup);
afterEach(() => server.resetHandlers());

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeDatasetResponse(overrides?: Partial<EvalDatasetResponse>): EvalDatasetResponse {
  return {
    dataset_name: "dep-test-deployment",
    item_count: 42,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// MSW helpers
// ---------------------------------------------------------------------------

function setupDataset(response: EvalDatasetResponse | { status: number }) {
  server.use(
    http.get("/api/v1/deployments/:id/dataset", () => {
      if ("status" in response) {
        return new HttpResponse(null, { status: response.status });
      }
      return HttpResponse.json(response);
    }),
  );
}

// ---------------------------------------------------------------------------
// Rendering helper
// ---------------------------------------------------------------------------

function renderDataset(deploymentId = "dep-test") {
  return renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: () => (
          <Outlet
            context={{
              deployment: { id: deploymentId },
              account: "testuser",
              deploymentId,
            }}
          />
        ),
        children: [{ path: "dataset", Component: AgentDataset }],
      },
    ],
    {
      initialEntries: [`/testuser/agents/${deploymentId}/dataset`],
      auth: mockAuthContext,
    },
  );
}

// ===========================================================================
// Tests
// ===========================================================================

describe("loading state", () => {
  it("shows a spinner while the request is in flight", () => {
    server.use(
      http.get("/api/v1/deployments/:id/dataset", () => new Promise(() => {})),
    );
    const { container } = renderDataset();
    expect(container.querySelector(".dp-spin")).toBeInTheDocument();
  });
});

describe("error state", () => {
  it("shows unavailable message on API error", async () => {
    setupDataset({ status: 404 });
    renderDataset();
    expect(await screen.findByText(/dataset not available/i)).toBeInTheDocument();
    expect(screen.queryByRole("article")).not.toBeInTheDocument();
  });
});

describe("summary view", () => {
  it("shows dataset name and item count", async () => {
    setupDataset(makeDatasetResponse());
    renderDataset();
    expect(await screen.findByText("dep-test-deployment")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
  });

  it("shows download button", async () => {
    setupDataset(makeDatasetResponse());
    renderDataset();
    expect(await screen.findByRole("link", { name: /download/i })).toBeInTheDocument();
  });

  it("does not show download button while loading", () => {
    server.use(
      http.get("/api/v1/deployments/:id/dataset", () => new Promise(() => {})),
    );
    renderDataset();
    expect(screen.queryByRole("link", { name: /download/i })).not.toBeInTheDocument();
  });

  it("formats large item counts with locale separators", async () => {
    setupDataset(makeDatasetResponse({ item_count: 12345 }));
    renderDataset();
    await waitFor(() => {
      expect(screen.getByText("12,345")).toBeInTheDocument();
    });
  });
});
