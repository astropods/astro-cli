import { createBrowserRouter, Navigate } from "react-router";
import { AppShell } from "@/components/layout/app-shell";

import { AccountsPage } from "@/pages/accounts";
import { DeploymentsPage } from "@/pages/deployments";
import { DeploymentDetailPage } from "@/pages/deployment-detail";
import { AgentsPage } from "@/pages/agents";
import { SqlQueryPage } from "@/pages/sql-query";
import { ClusterPage } from "@/pages/cluster";
import { ImagesPage } from "@/pages/images";
import { MetersPage } from "@/pages/meters";
import { FeaturesPage } from "@/pages/features";
import { CustomersPage } from "@/pages/customers";
import { CustomerDetailPage } from "@/pages/customer-detail";
import { EventsPage } from "@/pages/events";

export const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/admin/deployments" replace /> },
      { path: "admin/accounts", element: <AccountsPage /> },
      { path: "admin/deployments", element: <DeploymentsPage /> },
      { path: "admin/deployments/:namespace", element: <DeploymentDetailPage /> },
      { path: "admin/agents", element: <AgentsPage /> },
      { path: "admin/sql", element: <SqlQueryPage /> },
      { path: "admin/cluster", element: <ClusterPage /> },
      { path: "admin/images", element: <ImagesPage /> },
      { path: "openmeter/meters", element: <MetersPage /> },
      { path: "openmeter/features", element: <FeaturesPage /> },
      { path: "openmeter/customers", element: <CustomersPage /> },
      { path: "openmeter/customers/:id", element: <CustomerDetailPage /> },
      { path: "openmeter/events", element: <EventsPage /> },
    ],
  },
]);
