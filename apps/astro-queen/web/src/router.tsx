import { createBrowserRouter, Navigate } from "react-router";
import { AppShell } from "@/components/layout/app-shell";

import { AccountsPage } from "@/pages/accounts";
import { AccountDetailPage } from "@/pages/account-detail";
import { DeploymentsPage } from "@/pages/deployments";
import { DeploymentDetailPage } from "@/pages/deployment-detail";
import { BlueprintsPage } from "@/pages/blueprints";

import { ApiClientPage } from "@/pages/api-client";
import { JobsPage } from "@/pages/jobs";
import { QuotaRequestsPage } from "@/pages/quota-requests";
import { FeedbackPage } from "@/pages/feedback";
import { ClustersPage } from "@/pages/clusters";
import { MigrationsPage } from "@/pages/migrations";
import { AlertsPage } from "@/pages/alerts";
import { AuditPage } from "@/pages/audit";
import { ResourcesPage } from "@/pages/authorization";
import { ResourceDetailPage } from "@/pages/resource-detail";

export const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/admin/deployments" replace /> },
      { path: "admin/accounts", element: <AccountsPage /> },
      { path: "admin/accounts/:id", element: <AccountDetailPage /> },
      { path: "admin/deployments", element: <DeploymentsPage /> },
      { path: "admin/deployments/:id", element: <DeploymentDetailPage /> },
      { path: "admin/resources", element: <ResourcesPage /> },
      { path: "admin/resources/:type/:id", element: <ResourceDetailPage /> },
      { path: "admin/blueprints", element: <BlueprintsPage /> },

      { path: "admin/api-client", element: <ApiClientPage /> },
      { path: "admin/jobs", element: <JobsPage /> },
      { path: "admin/quota-requests", element: <QuotaRequestsPage /> },
      { path: "admin/clusters", element: <ClustersPage /> },
      { path: "admin/migrations", element: <MigrationsPage /> },
      { path: "admin/feedback", element: <FeedbackPage /> },
      { path: "admin/alerts", element: <AlertsPage /> },
      { path: "admin/audit", element: <AuditPage /> },
    ],
  },
]);
