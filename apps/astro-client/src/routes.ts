import { type RouteConfig, route, layout, index, prefix } from "@react-router/dev/routes";

export default [
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
    route(":account", "pages/AccountProfile.tsx"),
    route(":account/:agentSlug", "pages/BlueprintDetail.tsx", { id: "agent-detail" }),
    route("*", "pages/NotFound.tsx"),

    // Protected routes — auth is checked once at the layout level before any
    // child page mounts, so hooks in child components are safe.
    layout("components/ProtectedLayout.tsx", [
      route("getting-started", "pages/NewBlueprint.tsx", { id: "getting-started" }),
      route("new/custom", "pages/NewBlueprint.tsx", { id: "new-custom" }),
      route("agents", "pages/AgentDashboard.tsx"),
      route("knowledge", "pages/knowledge/KnowledgeStores.tsx"),
      route("knowledge/new", "pages/knowledge/NewKnowledgeStore/NewKnowledgeStore.tsx"),
      route("knowledge/:storeName", "pages/knowledge/KnowledgeStoreDetail.tsx"),
      route("admin", "pages/Admin.tsx"),
      ...prefix("settings", [
        layout("pages/settings/SettingsLayout.tsx", [
          index("pages/settings/SettingsRedirect.tsx"),
          route("account", "pages/settings/AccountSettings.tsx"),
          route("usage", "pages/settings/UsageSettings.tsx"),
          route("secrets", "pages/settings/SecretsSettings.tsx"),
          route("organizations", "pages/settings/OrganizationsSettings.tsx"),
          route("experiments", "pages/settings/ExperimentsSettings.tsx"),
          route("audit-log", "pages/settings/AuditLogSettings.tsx"),
        ]),
        ...prefix("org/:orgSlug", [
          layout("pages/settings/OrgSettingsLayout.tsx", [
            route("general", "pages/settings/OrgGeneralSettings.tsx"),
            route("members", "pages/settings/OrgMembersSettings.tsx"),
            route("secrets", "pages/settings/OrgSecretsSettings.tsx"),
            route("audit-log", "pages/settings/OrgAuditLogSettings.tsx"),
          ]),
        ]),
      ]),
      route("organization/new", "pages/OrganizationNew.tsx"),
      route("organization", "pages/OrganizationRedirect.tsx"),
      route("deploy/:account/:agentSlug", "pages/DeployBlueprint.tsx"),
      route(":account/agents/:deploymentId", "pages/DeployedAgentDetail.tsx"),
      ...prefix(":account/agents/:deploymentId/configure", [
        layout("pages/DeployedAgentSettings.tsx", [
          index("pages/configure/ConfigureRedirect.tsx"),
          route("deployment", "pages/configure/ConfigureDeployment.tsx"),
          route("danger-zone", "pages/configure/ConfigureDangerZone.tsx"),
        ]),
      ]),
    ]),
  ]),
] satisfies RouteConfig;
