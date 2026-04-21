import { describe, it, expect, beforeAll, beforeEach, afterEach } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/test-utils";
import { DeploymentsTab } from "./DeploymentsTab";
import type { AgentDeployment } from "@/lib/api";

afterEach(cleanup);

// jsdom does not implement matchMedia
beforeAll(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
});

function makeDeployment(env: { name: string; value?: string; from?: string }[]): AgentDeployment {
  return {
    id: "dep-1",
    name: "test-agent",
    build_id: "abc123",
    namespace: "astro-abc123",
    status: "Running",
    replicas: 1,
    ready: 1,
    created_at: "2026-01-01T00:00:00Z",
    components: ["agent"],
    workloads: [
      {
        name: "test-agent-workload",
        kind: "Deployment",
        component: "agent",
        age: "1d",
        containers: [{ name: "app", state: "Running", ready: true, restart_count: 0, env }],
      },
    ],
  };
}

beforeEach(() => {
  server.use(
    http.get("/api/v1/agents/:account/:name/deployment/history", () =>
      HttpResponse.json({ deployments: [], count: 0 })
    )
  );
});

describe("DeploymentsTab — env var ordering", () => {
  it("sorts env vars alphabetically by key regardless of server order", async () => {
    const deployment = makeDeployment([
      { name: "ZEBRA_VAR", value: "z" },
      { name: "ALPHA_VAR", value: "a" },
      { name: "MIDDLE_VAR", value: "m" },
    ]);

    renderWithProviders(<DeploymentsTab deployment={deployment} account="testuser" />);

    // Accordion auto-opens on mount — wait for var keys to appear
    const alphaEl = await screen.findByText("ALPHA_VAR");
    const middleEl = screen.getByText("MIDDLE_VAR");
    const zebraEl = screen.getByText("ZEBRA_VAR");

    // Verify DOM order: each key must precede the next
    expect(alphaEl.compareDocumentPosition(middleEl) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(middleEl.compareDocumentPosition(zebraEl) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("sorts env vars from multiple sources together by key", async () => {
    const deployment = makeDeployment([
      { name: "SECRET_KEY", value: "••••••••", from: "secret:my-secret" },
      { name: "CONFIG_VAL", value: "hello", from: "configmap:my-config" },
      { name: "ANOTHER_VAR", value: "world" },
    ]);

    renderWithProviders(<DeploymentsTab deployment={deployment} account="testuser" />);

    const anotherEl = await screen.findByText("ANOTHER_VAR");
    const configEl = screen.getByText("CONFIG_VAL");
    const secretEl = screen.getByText("SECRET_KEY");

    expect(anotherEl.compareDocumentPosition(configEl) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(configEl.compareDocumentPosition(secretEl) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
