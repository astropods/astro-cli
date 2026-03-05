import { createBrowserRouter, Navigate } from "react-router";
import { AppShell } from "@/components/layout/app-shell";

import { AccountsPage } from "@/pages/accounts";
import { DeploymentsPage } from "@/pages/deployments";
import { DeploymentDetailPage } from "@/pages/deployment-detail";
import { AgentsPage } from "@/pages/agents";
import { ClusterPage } from "@/pages/cluster";
import { MetersPage } from "@/pages/meters";
import { FeaturesPage } from "@/pages/features";
import { CustomersPage } from "@/pages/customers";
import { CustomerDetailPage } from "@/pages/customer-detail";
import { EventsPage } from "@/pages/events";
import { OpenMeterHomePage } from "@/pages/openmeter-home";

export const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/openmeter" replace /> },
      { path: "openmeter", element: <OpenMeterHomePage /> },
      { path: "admin/accounts", element: <AccountsPage /> },
      { path: "admin/deployments", element: <DeploymentsPage /> },
      { path: "admin/deployments/:namespace", element: <DeploymentDetailPage /> },
      { path: "admin/agents", element: <AgentsPage /> },
      { path: "admin/cluster", element: <ClusterPage /> },
      { path: "openmeter/meters", element: <MetersPage /> },
      { path: "openmeter/features", element: <FeaturesPage /> },
      { path: "openmeter/customers", element: <CustomersPage /> },
      { path: "openmeter/customers/:id", element: <CustomerDetailPage /> },
      { path: "openmeter/events", element: <EventsPage /> },
    ],
  },
]);
