import { describe, expect, it } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/test-utils";
import { ChatDeploymentSummary, ChatInspectorPanel } from "./ChatInspectorPanel";

describe("ChatInspectorPanel", () => {
  it("links View agent to the monitor tab", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "inactive",
          reason: "paused",
          details: "paused",
        }),
      ),
    );

    renderWithProviders(
      <ChatInspectorPanel
        account="acme"
        deploymentId="dep-1"
        deployment={{
          id: "dep-1",
          name: "test-agent",
          display_name: "Test Agent",
          build_id: "build-1",
          created_at: "2026-01-01T00:00:00Z",
          messaging_web_configured: true,
        }}
        tab="settings"
        onTabChange={() => {}}
        onClose={() => {}}
      />,
    );

    expect(
      await screen.findByRole("link", { name: /view agent/i }),
    ).toHaveAttribute("href", "/acme/agents/dep-1/monitor");
    const agentName = screen.getByText("Test Agent");
    const settingsTab = screen.getByRole("button", { name: "Config" });
    expect(
      agentName.compareDocumentPosition(settingsTab) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("shows recent trace status, user, date, tokens, and cost", async () => {
    const traceRequestUrls: string[] = [];
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "active",
          reason: "ready",
          details: "ready",
        }),
      ),
      http.get("/api/v1/deployments/:id", () =>
        HttpResponse.json({
          deployment: {
            id: "dep-1",
            name: "test-agent",
            display_name: "Test Agent",
            build_id: "build-1",
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-06-01T10:00:00Z",
            messaging_configured: true,
            workloads: [],
          },
        }),
      ),
      http.get("/api/v1/deployments/:id/runtime", () =>
        HttpResponse.json({
          runtime: {
            ready: 0,
            replicas: 0,
            messaging_reachable: true,
            workloads: [],
          },
        }),
      ),
      http.get("/api/v1/deployments/:id/observability/summary", () =>
        HttpResponse.json({
          total_traces: 2,
          time_range: {
            start: "2026-05-25T00:00:00Z",
            end: "2026-06-01T12:00:00Z",
          },
          metrics: {
            avg_latency_ms: 500,
            p95_latency_ms: 900,
            total_tokens: 1480,
            error_rate: 0,
            traces_per_hour: 1,
          },
        }),
      ),
      http.get("/api/v1/accounts/:account/observability/deployment-summaries", () =>
        HttpResponse.json({
          summaries: {
            "dep-1": {
              total_traces: 2,
              last_trace_at: "2026-06-01T12:15:00Z",
              cost_usd: 0.8,
              request_series: [1, 2, 3],
              token_series: [400, 500, 580],
              cost_series: [0.1, 0.2, 0.5],
            },
          },
        }),
      ),
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        traceRequestUrls.push(request.url);
        return HttpResponse.json({
          total: 1,
          limit: 6,
          offset: 0,
          traces: [
            {
              trace_id: "trace-refund-7f4a2c91",
              name: "refund eligibility review",
              status: "error",
              latency_ms: 642,
              total_tokens: 1480,
              total_cost: 0.0042,
              input: "Customer asks for a refund.",
              output: "Escalated for review.",
              timestamp: "2026-06-01T12:15:00Z",
              user_id: "slack:U013TEST",
              user_details: {
                kind: "slack",
                display_name: "Maya Chen",
                username: "maya.chen",
              },
            },
          ],
        });
      }),
    );

    renderWithProviders(
      <ChatInspectorPanel
        account="acme"
        deploymentId="dep-1"
        deployment={{
          id: "dep-1",
          name: "test-agent",
          display_name: "Test Agent",
          build_id: "build-1",
          created_at: "2026-01-01T00:00:00Z",
          messaging_web_configured: true,
        }}
        tab="overview"
        onTabChange={() => {}}
        onClose={() => {}}
      />,
    );

    expect(await screen.findByText("Maya Chen")).toBeInTheDocument();
    expect(screen.getByText("$0.80")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "30D" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    fireEvent.click(screen.getByRole("button", { name: "7D" }));
    expect(screen.getByRole("button", { name: "7D" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(traceRequestUrls.length).toBeGreaterThan(0);
    for (const requestUrl of traceRequestUrls) {
      const url = new URL(requestUrl);
      expect(url.searchParams.get("limit")).toBe("8");
      expect(url.searchParams.has("start_time")).toBe(false);
      expect(url.searchParams.has("end_time")).toBe(false);
    }
    expect(screen.queryByText("Spend / req")).not.toBeInTheDocument();
    expect(screen.queryByText("$0.40")).not.toBeInTheDocument();
    expect(screen.queryByText("Trace rate")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "View all traces" }),
    ).toHaveAttribute("href", "/acme/agents/dep-1/monitor#traces");
    expect(screen.queryByText("P95 latency")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Error")).toBeInTheDocument();
    const cost = screen.getByText("$0.0042");
    const tokens = screen.getByText("1.5K tokens");
    expect(cost).toBeInTheDocument();
    expect(tokens).toBeInTheDocument();
    expect(
      cost.compareDocumentPosition(tokens) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getByText(/Jun 1/)).toBeInTheDocument();
    expect(screen.queryByText(/^Mon,/)).not.toBeInTheDocument();
  });

  it("summarizes deployment status without build metadata", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "active",
          reason: "ready",
          details: "ready",
        }),
      ),
      http.get("/api/v1/deployments/:id/observability/summary", () =>
        HttpResponse.json({
          total_traces: 0,
          time_range: {
            start: "2026-05-07T00:00:00Z",
            end: "2026-06-06T00:00:00Z",
          },
          metrics: {
            avg_latency_ms: 0,
            p95_latency_ms: 0,
            total_tokens: 0,
            error_rate: 0,
            traces_per_hour: 0,
          },
        }),
      ),
      http.get("/api/v1/accounts/:account/observability/deployment-summaries", () =>
        HttpResponse.json({
          summaries: {
            "dep-1": {
              total_traces: 0,
              request_series: [0, 0, 0],
              token_series: [0, 0, 0],
              cost_series: [0, 0, 0],
            },
          },
        }),
      ),
      http.get("/api/v1/deployments/:id/observability/traces", () =>
        HttpResponse.json({
          total: 0,
          limit: 6,
          offset: 0,
          traces: [],
        }),
      ),
    );

    renderWithProviders(
      <ChatInspectorPanel
        account="acme"
        deploymentId="dep-1"
        deployment={{
          id: "dep-1",
          name: "test-agent",
          display_name: "Test Agent",
          build_id: "mockb17a",
          latest_build_id: "mockb18b",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-06-01T10:00:00Z",
          messaging_web_configured: true,
        }}
        tab="overview"
        onTabChange={() => {}}
        onClose={() => {}}
      />,
    );

    expect(await screen.findByText("Active")).toBeInTheDocument();
    expect(screen.queryByText("Deployment")).not.toBeInTheDocument();
    expect(screen.getByText("Active")).not.toHaveClass("animate-pulse");
    expect(screen.queryByText("mockb17a")).not.toBeInTheDocument();
    expect(screen.queryByText("Update")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /update to new build/i }))
      .not.toBeInTheDocument();
  });

  it("collapses deployment statuses into chat-facing labels", () => {
    renderWithProviders(
      <div>
        <ChatDeploymentSummary
          status={{
            value: "deploying",
            reason: "provisioning",
            details: "Pods are being provisioned",
          }}
        />
        <ChatDeploymentSummary
          status={{
            value: "undeploying",
            reason: "undeploying",
            details: "Deployment is being torn down",
          }}
        />
        <ChatDeploymentSummary
          status={{
            value: "inactive",
            reason: "paused",
            details: "Paused",
          }}
        />
        <ChatDeploymentSummary
          status={{
            value: "error",
            reason: "failed",
            details: "Failed",
          }}
        />
      </div>,
    );

    expect(screen.getAllByText("Deploying")).toHaveLength(2);
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.queryByText("Preparing")).not.toBeInTheDocument();
    expect(screen.queryByText("Building")).not.toBeInTheDocument();
    expect(screen.queryByText("Undeploying")).not.toBeInTheDocument();
  });
});
