import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { renderRoute } from "@/test/test-utils";
import type { AgentDeploymentSummary, DeploymentSummaryStatus } from "@/lib/api";
import { DeploymentAgentCard } from "./DeploymentAgentCard";

afterEach(cleanup);

const BASE: AgentDeploymentSummary = {
  id: "dep-1",
  name: "my-agent",
  build_id: "build-1",
  created_at: "2025-01-01T00:00:00Z",
  external_urls: [{ name: "messaging", type: "messaging", url: "https://my-agent.example.com", ready: true }],
};

function renderCard(deployment: AgentDeploymentSummary) {
  return renderRoute(
    [{ path: "/", Component: () => <DeploymentAgentCard deployment={deployment} account="testuser" /> }],
    { auth: null },
  );
}

describe("DeploymentAgentCard launch button", () => {
  it("shows Launch when the deployment is running", () => {
    renderCard({ ...BASE, status: "Running" });
    expect(screen.getByRole("link", { name: /launch/i })).toBeInTheDocument();
  });

  it.each<DeploymentSummaryStatus>(["pending", "Stopped", "undeploying", "error"])(
    "hides Launch when status is %s",
    (status) => {
      renderCard({ ...BASE, status });
      expect(screen.queryByRole("link", { name: /launch/i })).not.toBeInTheDocument();
    },
  );

  it("hides Launch when there is no messaging URL", () => {
    renderCard({ ...BASE, status: "Running", external_urls: [] });
    expect(screen.queryByRole("link", { name: /launch/i })).not.toBeInTheDocument();
  });
});
