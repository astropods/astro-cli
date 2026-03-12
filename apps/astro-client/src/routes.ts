import { type RouteConfig, route, layout, index, prefix } from "@react-router/dev/routes";

export default [
  layout("components/Layout.tsx", [
    index("pages/Index.tsx"),
    route("browse", "pages/Hire.tsx"),
    route("request-agent", "pages/RequestAgent.tsx"),
    route("agents", "pages/YourAgents.tsx"),
    route("onboarding", "pages/Onboarding.tsx"),
    route("admin", "pages/Admin.tsx"),
    ...prefix("settings", [
      layout("pages/settings/SettingsLayout.tsx", [
        index("pages/settings/SettingsRedirect.tsx"),
        route("account", "pages/settings/AccountSettings.tsx"),
      ]),
    ]),
    route("organization/new", "pages/OrganizationNew.tsx"),
    route("organization", "pages/OrganizationRedirect.tsx"),
    route("deploy/:account/:agentSlug", "pages/InstallAgent.tsx"),
    route(":account", "pages/AccountProfile.tsx"),
    route(":account/agents/:deploymentId", "pages/DeployedAgentDetail.tsx"),
    ...prefix(":account/agents/:deploymentId/configure", [
      layout("pages/DeployedAgentSettings.tsx", [
        index("pages/configure/ConfigureRedirect.tsx"),
        route("deployment", "pages/configure/ConfigureDeployment.tsx"),
        route("danger-zone", "pages/configure/ConfigureDangerZone.tsx"),
      ]),
    ]),
    route(":account/:agentSlug", "pages/AgentDetail.tsx", { id: "agent-detail" }),
    route("*", "pages/NotFound.tsx"),
  ]),
] satisfies RouteConfig;
