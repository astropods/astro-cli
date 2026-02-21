import { type RouteConfig, route, layout, index } from "@react-router/dev/routes";

export default [
  layout("components/Layout.tsx", [
    index("pages/Home.tsx"),
    route("hire", "pages/Hire.tsx"),
    route("request-agent", "pages/RequestAgent.tsx"),
    route("agents", "pages/YourAgents.tsx"),
    route("operator", "pages/OperatorOverview.tsx"),
    route("operator/deploy/:account/:name", "pages/DeployPage.tsx"),
    route("u/:account/:agent", "pages/AgentPage.tsx"),
    route("onboarding", "pages/Onboarding.tsx"),
    route("docs", "pages/Docs.tsx"),
    route("dev", "pages/DevRedirect.tsx"),
    route("deploy/:account/:agentSlug", "pages/InstallAgent.tsx"),
    route(":account/:agentSlug", "pages/AgentDetail.tsx", { id: "agent-detail" }),
    route("admin", "pages/Admin.tsx"),
    route("*", "pages/NotFound.tsx"),
  ]),
] satisfies RouteConfig;
