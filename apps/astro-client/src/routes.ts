import { type RouteConfig, route, layout, index } from "@react-router/dev/routes";

export default [
  layout("components/Layout.tsx", [
    index("pages/Index.tsx"),
    route("browse", "pages/Hire.tsx"),
    route("request-agent", "pages/RequestAgent.tsx"),
    route("agents", "pages/YourAgents.tsx"),
    route("operator", "pages/legacy/OperatorOverview.tsx"),
    route("operator/deploy/:account/:name", "pages/legacy/DeployPage.tsx"),
    route("u/:account/:agent", "pages/legacy/AgentPage.tsx"),
    route("onboarding", "pages/Onboarding.tsx"),
    route("deploy/:account/:agentSlug", "pages/InstallAgent.tsx"),
    route(":account/:agentSlug", "pages/AgentDetail.tsx", { id: "agent-detail" }),
    route("*", "pages/NotFound.tsx"),
  ]),
] satisfies RouteConfig;
