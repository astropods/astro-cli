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

function deployment(overrides: Partial<AgentDeployment> = {}): AgentDeployment {
  return {
    id: DEPLOYMENT_ID,
    name: AGENT,
    build_id: "b2c3d4e5",
    namespace: "ns",
    status: "running",
    replicas: 1,
    created_at: "2026-05-01T00:00:00Z",
    components: ["agent"],
    ...overrides,
  };
}

function historyRecord(overrides: Partial<DeploymentHistoryRecord> = {}): DeploymentHistoryRecord {
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
    ...overrides,
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

describe("DeploymentHistoryPanel", () => {
  it("resolves the branch-labeled commit link against the repository root", async () => {
    mockEndpoints([]);
    server.use(
      http.get(`/api/v1/agents/${ACCOUNT}/${AGENT}/deployment/history`, () =>
        HttpResponse.json({
          deployments: [
            historyRecord({
              repo_full_name: "acme/monorepo/agents/code-reviewer",
              commit_sha: "abc1234def",
            }),
          ],
          count: 1,
        }),
      ),
    );

    renderPanel();

    expect(await screen.findByRole("link", { name: "main" })).toHaveAttribute(
      "href",
      "https://github.com/acme/monorepo/commit/abc1234def",
    );
  });

  it("shows the account member who initiated the active deployment", async () => {
    mockEndpoints([]);
    server.use(
      http.get(`/api/v1/accounts/${ACCOUNT}/members`, () =>
        HttpResponse.json({
          members: [
            {
              account_id: "acct-1",
              user_id: "user-taylor",
              role: "member",
              status: "active",
              username: "taylor",
              display_name: "Taylor Kim",
              avatar_url: "https://example.com/taylor.png",
              created_at: "2026-01-01T00:00:00Z",
              slack_workspaces: [],
            },
          ],
        }),
      ),
    );

    renderWithProviders(
      <DeploymentHistoryPanel
        account={ACCOUNT}
        agentName={AGENT}
        deploymentId={DEPLOYMENT_ID}
        deployment={deployment({ deployed_by: "user-taylor" })}
      />,
    );

    expect(await screen.findByText("Taylor Kim")).toBeInTheDocument();
    expect(screen.getByAltText("Taylor Kim")).toHaveAttribute(
      "src",
      "https://example.com/taylor.png",
    );
    expect(screen.getByText("Taylor Kim").closest("a")).toHaveAttribute(
      "href",
      "/taylor",
    );
  });

  it.each(["admin:grpc", "user-removed"])(
    "hides unresolved %s audit actors instead of rendering internal IDs",
    async (actorId) => {
      mockEndpoints([]);
      renderWithProviders(
        <DeploymentHistoryPanel
          account={ACCOUNT}
          agentName={AGENT}
          deploymentId={DEPLOYMENT_ID}
          deployment={deployment({ deployed_by: actorId })}
        />,
      );

      await waitFor(() => {
        expect(screen.getByText("Code Reviewer")).toBeInTheDocument();
      });
      expect(screen.queryByText(actorId)).not.toBeInTheDocument();
      expect(screen.queryByAltText(actorId)).not.toBeInTheDocument();
    },
  );

  it("shows deployment author avatars for active and previous revisions", async () => {
    mockEndpoints([]);
    server.use(
      http.get(`/api/v1/agents/${ACCOUNT}/${AGENT}/deployment/history`, () =>
        HttpResponse.json({
          deployments: [
            historyRecord({ revision: 2, deployed_by: "user-taylor" }),
            historyRecord({
              revision: 1,
              is_current: false,
              deployed_at: "2026-04-01T00:00:00Z",
              deployed_by: "user-jordan",
            }),
          ],
          count: 2,
        }),
      ),
      http.get(`/api/v1/accounts/${ACCOUNT}/members`, () =>
        HttpResponse.json({
          members: [
            {
              account_id: "acct-1",
              user_id: "user-taylor",
              role: "member",
              status: "active",
              username: "taylor",
              display_name: "Taylor Kim",
              avatar_url: "https://example.com/taylor.png",
              created_at: "2026-01-01T00:00:00Z",
              slack_workspaces: [],
            },
            {
              account_id: "acct-1",
              user_id: "user-jordan",
              role: "member",
              status: "active",
              username: "jordan",
              display_name: "Jordan Lee",
              avatar_url: "https://example.com/jordan.png",
              created_at: "2026-01-01T00:00:00Z",
              slack_workspaces: [],
            },
          ],
        }),
      ),
    );

    renderWithProviders(
      <DeploymentHistoryPanel
        account={ACCOUNT}
        agentName={AGENT}
        deploymentId={DEPLOYMENT_ID}
        deployment={deployment()}
        expanded
      />,
    );

    expect(await screen.findByAltText("Taylor Kim")).toHaveAttribute(
      "src",
      "https://example.com/taylor.png",
    );
    expect(screen.getByAltText("Jordan Lee")).toHaveAttribute(
      "src",
      "https://example.com/jordan.png",
    );
    expect(screen.getByText("Jordan Lee").closest("a")).toHaveAttribute(
      "href",
      "/jordan",
    );
  });
});

describe("DeploymentHistoryPanel build-in-progress card", () => {
  it("shows a live building card with the commit message while the latest build is running", async () => {
    mockEndpoints([build({ status: "building" })]);
    const { container } = renderPanel();

    expect(await screen.findByText("Pushing new build")).toBeInTheDocument();
    expect(screen.getByText("Add retries with backoff")).toBeInTheDocument();
    // The build card spins, distinct from the static active tile below it.
    expect(container.querySelector(".animate-spin")).toBeTruthy();
  });

  it("shows a preparing card while the latest build is queued", async () => {
    mockEndpoints([build({ status: "pending" })]);
    const { container } = renderPanel();

    expect(await screen.findByText("Preparing build")).toBeInTheDocument();
    expect(container.querySelector(".animate-spin")).toBeTruthy();
  });

  it("suppresses the build card while the deployment is actively deploying (no stacking)", async () => {
    server.use(
      http.get(`/api/v1/deployments/${DEPLOYMENT_ID}/status`, () =>
        HttpResponse.json({ value: "deploying", reason: "provisioning", details: "Pods are being provisioned" }),
      ),
    );
    mockEndpoints([build({ status: "building" })]);
    renderPanel();

    // The tile's Deploying status is the single in-progress indicator; the
    // build card must not stack above it.
    await waitFor(() => {
      expect(screen.getByText("Code Reviewer")).toBeInTheDocument();
    });
    expect(screen.queryByText("Pushing new build")).not.toBeInTheDocument();
  });

  it("shows no build-in-progress card once the latest build has finished", async () => {
    mockEndpoints([build({ status: "registered" })]);
    renderPanel();

    // The active deployment tile still renders.
    await waitFor(() => {
      expect(screen.getByText("Code Reviewer")).toBeInTheDocument();
    });
    expect(screen.queryByText("Pushing new build")).not.toBeInTheDocument();
    expect(screen.queryByText("Preparing build")).not.toBeInTheDocument();
  });

  it("surfaces a finished build as an available upgrade instead of letting it vanish (#1627)", async () => {
    // Finished build newer than the deployed one, no published blueprint version
    // yet: it must still surface as an available upgrade rather than disappear.
    mockEndpoints([build({ status: "registered", build_id: "newbuild" })]);
    renderPanel();

    expect(await screen.findByText("New build available")).toBeInTheDocument();
    expect(screen.getByText("Add retries with backoff")).toBeInTheDocument();
    expect(screen.queryByText("Pushing new build")).not.toBeInTheDocument();
  });

  it("shows a Latest build badge when there is nothing newer to deploy (#1627)", async () => {
    // GitHub-sourced, no in-flight build and no upgrade: the header reassures
    // the user they are current instead of falling silent.
    mockEndpoints([]);
    renderPanel();

    expect(await screen.findByText("Latest build")).toBeInTheDocument();
    expect(screen.queryByText("New build available")).not.toBeInTheDocument();
    expect(screen.queryByText("Pushing new build")).not.toBeInTheDocument();
  });

  it("renders the available build's branch like the active tile (#1629)", async () => {
    // The finished build is on "main"; the nudge must show the branch the same
    // way the active tile does, not just a bare commit sha. So "main" now
    // appears twice: on the active tile and on the nudge.
    mockEndpoints([build({ status: "registered", build_id: "newbuild" })]);
    renderPanel();

    await screen.findByText("New build available");
    expect(screen.getAllByText("main").length).toBeGreaterThanOrEqual(2);
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
