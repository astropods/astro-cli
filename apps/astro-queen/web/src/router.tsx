import { createBrowserRouter, Navigate } from "react-router";
import { AppShell } from "@/components/layout/app-shell";

import { AccountsPage } from "@/pages/accounts";
import { DeploymentsPage } from "@/pages/deployments";
import { DeploymentDetailPage } from "@/pages/deployment-detail";
import { BlueprintsPage } from "@/pages/blueprints";

import { ConnectedDevicesPage } from "@/pages/connected-devices";
import { ApiClientPage } from "@/pages/api-client";
import { MetersPage } from "@/pages/meters";
import { FeaturesPage } from "@/pages/features";
import { CustomersPage } from "@/pages/customers";
import { CustomerDetailPage } from "@/pages/customer-detail";
import { EventsPage } from "@/pages/events";
import { PlansPage } from "@/pages/plans";
import { OpenMeterHomePage } from "@/pages/openmeter-home";
import { OpenMeterDashboardPage } from "@/pages/openmeter-dashboard";
import { RiverUIPage } from "@/pages/river-ui";
import { QuotaRequestsPage } from "@/pages/quota-requests";
import { FeedbackPage } from "@/pages/feedback";
import { ClustersPage } from "@/pages/clusters";

export const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/openmeter" replace /> },
      { path: "openmeter", element: <OpenMeterHomePage /> },
      { path: "admin/accounts", element: <AccountsPage /> },
      { path: "admin/deployments", element: <DeploymentsPage /> },
      { path: "admin/deployments/:id", element: <DeploymentDetailPage /> },
      { path: "admin/blueprints", element: <BlueprintsPage /> },

      { path: "admin/devices", element: <ConnectedDevicesPage /> },
      { path: "admin/api-client", element: <ApiClientPage /> },
      { path: "admin/river-ui", element: <RiverUIPage /> },
      { path: "admin/quota-requests", element: <QuotaRequestsPage /> },
      { path: "admin/clusters", element: <ClustersPage /> },
      { path: "admin/feedback", element: <FeedbackPage /> },
      { path: "openmeter/dashboard", element: <OpenMeterDashboardPage /> },
      { path: "openmeter/meters", element: <MetersPage /> },
      { path: "openmeter/features", element: <FeaturesPage /> },
      { path: "openmeter/customers", element: <CustomersPage /> },
      { path: "openmeter/customers/:id", element: <CustomerDetailPage /> },
      { path: "openmeter/plans", element: <PlansPage /> },
      { path: "openmeter/events", element: <EventsPage /> },
    ],
  },
]);
