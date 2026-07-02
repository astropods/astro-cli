import { describe, it, expect, afterEach } from "vitest";
import { screen, waitFor, cleanup } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { renderWithProviders } from "@/test/test-utils";
import { server } from "@/test/msw/server";
import { DeploymentHistoryPanel } from "./DeploymentHistoryPanel";
import type { AgentDeployment, DeploymentHistoryRecord, GitHubBuild } from "@/lib/api";

afterEach(cleanup);

const ACCOUNT = "testuser";
const AGENT = "code-reviewer";
const DEPLOYMENT_ID = "dep-1";

function deployment(): AgentDeployment {
  return {
    id: DEPLOYMENT_ID,
    name: AGENT,
    build_id: "b2c3d4e5",
    namespace: "ns",
    status: "running",
    replicas: 1,
    created_at: "2026-05-01T00:00:00Z",
    components: ["agent"],
  };
}

function historyRecord(): DeploymentHistoryRecord {
  return {
    id: DEPLOYMENT_ID,
    agent_name: AGENT,
    revision: 1,
    build_id: "b2c3d4e5",
    namespace: "ns",
    display_name: "Code Reviewer",
    is_current: true,
    status: "running",
    deployed_at: "2026-05-01T00:00:00Z",
    spec: {},
    source: "github",
    branch: "main",
  };
}

function build(overrides: Partial<GitHubBuild> = {}): GitHubBuild {
  return {
    id: "build-1",
    build_id: "newbuild",
    commit_sha: "abc1234def",
    branch: "main",
    status: "building",
    commit_message: "Add retries with backoff",
    enqueued_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

function mockEndpoints(builds: GitHubBuild[]) {
  server.use(
    http.get(`/api/v1/agents/${ACCOUNT}/${AGENT}/deployment/history`, () =>
      HttpResponse.json({ deployments: [historyRecord()], count: 1 }),
    ),
    http.get(`/api/v1/agents/${ACCOUNT}/${AGENT}/github`, () =>
      HttpResponse.json({
        connected: true,
        repo_full_name: "acme/code-reviewer",
        branch: "main",
        builds,
      }),
    ),
    // No newer published build, so the upgrade nudge stays out of the way.
    http.get(`/api/v1/agents/${ACCOUNT}`, () =>
      HttpResponse.json({ agents: [], total: 0 }),
    ),
  );
}

function renderPanel() {
  return renderWithProviders(
    <DeploymentHistoryPanel
      account={ACCOUNT}
      agentName={AGENT}
      deploymentId={DEPLOYMENT_ID}
      deployment={deployment()}
    />,
  );
}

describe("DeploymentHistoryPanel build-in-progress card", () => {
  it("shows a Building card with the commit message while the latest build is running", async () => {
    mockEndpoints([build({ status: "building" })]);
    renderPanel();

    expect(await screen.findByText("Building")).toBeInTheDocument();
    expect(screen.getByText("Add retries with backoff")).toBeInTheDocument();
  });

  it("shows a Preparing card while the latest build is queued", async () => {
    mockEndpoints([build({ status: "pending" })]);
    renderPanel();

    expect(await screen.findByText("Preparing")).toBeInTheDocument();
  });

  it("shows no build-in-progress card once the latest build has finished", async () => {
    mockEndpoints([build({ status: "registered" })]);
    renderPanel();

    // The active deployment tile still renders.
    await waitFor(() => {
      expect(screen.getByText("Code Reviewer")).toBeInTheDocument();
    });
    expect(screen.queryByText("Building")).not.toBeInTheDocument();
    expect(screen.queryByText("Preparing")).not.toBeInTheDocument();
  });

  it("does not request GitHub status for a cross-account private blueprint", async () => {
    const SOURCE = "publisher";
    let githubRequested = false;
    server.use(
      http.get(`/api/v1/agents/${ACCOUNT}/${AGENT}/deployment/history`, () =>
        HttpResponse.json({ deployments: [historyRecord()], count: 1 }),
      ),
      // The source account's blueprint is private to this viewer.
      http.get(`/api/v1/agents/${SOURCE}`, () =>
        HttpResponse.json({
          agents: [{ name: AGENT, visibility: "private", versions: [] }],
          total: 1,
        }),
      ),
      // A build is "in flight", but the query must stay disabled so this is
      // never requested.
      http.get(`/api/v1/agents/${SOURCE}/${AGENT}/github`, () => {
        githubRequested = true;
        return HttpResponse.json({
          connected: true,
          repo_full_name: `${SOURCE}/code-reviewer`,
          branch: "main",
          builds: [build({ status: "building" })],
        });
      }),
    );

    renderWithProviders(
      <DeploymentHistoryPanel
        account={ACCOUNT}
        agentName={AGENT}
        deploymentId={DEPLOYMENT_ID}
        deployment={{ ...deployment(), source_account: SOURCE }}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Code Reviewer")).toBeInTheDocument();
    });
    expect(githubRequested).toBe(false);
    expect(screen.queryByText("Building")).not.toBeInTheDocument();
  });
});
