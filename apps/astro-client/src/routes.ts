import { type RouteConfig, route, layout, index, prefix } from "@react-router/dev/routes";

export default [
  // Resource route — returns a PNG, not HTML, so lives outside the layout.
  route("badge/agents/*", "pages/BadgeAgent.tsx"),

  layout("components/Layout.tsx", [
    // Auth redirects
    route("login", "pages/Login.tsx"),
    route("signup", "pages/Signup.tsx"),

    // Public routes
    index("pages/Index.tsx"),
    route("blueprints", "pages/blueprints/Blueprints.tsx"),
    route("explore", "pages/Explore.tsx"),
    route("request-agent", "pages/RequestBlueprint.tsx"),
    route("onboarding", "pages/Onboarding.tsx"),
    route(":account", "pages/AccountProfile/AccountProfile.tsx"),
    route(":account/:agentSlug", "pages/BlueprintDetail.tsx", { id: "agent-detail" }),
    route("*", "pages/NotFound.tsx"),

    // Protected routes — auth is checked once at the layout level before any
    // child page mounts, so hooks in child components are safe.
    layout("components/ProtectedLayout.tsx", [
      route("getting-started", "pages/NewBlueprint.tsx", { id: "getting-started" }),
      route("new/custom", "pages/NewBlueprint.tsx", { id: "new-custom" }),
      route("agents", "pages/AgentDashboard.tsx"),
      route("insights", "pages/Insights.tsx"),
      route("knowledge", "pages/knowledge/KnowledgeStores.tsx"),
      route("knowledge/new", "pages/knowledge/NewKnowledgeStore/NewKnowledgeStore.tsx"),
      route("knowledge/:storeName", "pages/knowledge/KnowledgeStoreDetail/KnowledgeStoreDetail.tsx"),
      route("admin", "pages/Admin.tsx"),
      ...prefix("settings", [
        layout("pages/settings/SettingsLayout.tsx", [
          index("pages/settings/SettingsRedirect.tsx"),
          route("account", "pages/settings/AccountSettings.tsx"),
          route("usage", "pages/settings/UsageSettings.tsx"),
          route("secrets", "pages/settings/SecretsSettings.tsx"),
          route("connectors", "pages/settings/ConnectorsSettings.tsx"),
          route("organizations", "pages/settings/OrganizationsSettings.tsx"),
          route("experiments", "pages/settings/ExperimentsSettings.tsx"),
          route("audit-log", "pages/settings/AuditLogSettings.tsx"),
        ]),
        ...prefix("org/:orgSlug", [
          layout("pages/settings/OrgSettingsLayout.tsx", [
            route("general", "pages/settings/OrgGeneralSettings.tsx"),
            route("members", "pages/settings/OrgMembersSettings.tsx"),
            route("usage", "pages/settings/OrgUsageSettings.tsx"),
            route("secrets", "pages/settings/OrgSecretsSettings.tsx"),
            route("audit-log", "pages/settings/OrgAuditLogSettings.tsx"),
          ]),
        ]),
      ]),
      route("organization/new", "pages/OrganizationNew.tsx"),
      route("organization", "pages/OrganizationRedirect.tsx"),
      route("deploy/:account/:agentSlug", "pages/DeployBlueprint.tsx"),
      ...prefix(":account/agents/:deploymentId", [
        layout("pages/AgentDetail.tsx", [
          index("pages/agent-detail/AgentDetailRedirect.tsx"),
          route("monitor", "pages/agent-detail/AgentMonitor.tsx"),
          route("deployments", "pages/agent-detail/AgentDeployments.tsx"),
          route("dataset", "pages/agent-detail/AgentDataset.tsx"),
          route("configure", "pages/agent-detail/AgentConfigure.tsx"),
        ]),
      ]),
    ]),
  ]),
] satisfies RouteConfig;
