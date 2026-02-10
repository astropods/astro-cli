import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router-dom";

import { AppSidebar } from "@/components/AppSidebar";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { BreadcrumbHeader } from "@/components/BreadcrumbHeader";
import { PauseIcon } from "@heroicons/react/24/solid";
import { AgentHeader } from "@/components/AgentHeader";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";

function ContentAreaWithBreadcrumbs({
  children,
}: {
  children?: React.ReactNode;
}) {
  return (
    <SidebarInset>
      <BreadcrumbHeader
        breadcrumbs={[
          { label: "Hire Agents", to: "/hire" },
          { label: "Customer Insights Engine" },
        ]}
        onBack={() => {}}
        onForward={() => {}}
      />
      <div className="flex-1 p-6 md:p-8">{children}</div>
    </SidebarInset>
  );
}

function ContentAreaWithAgentHeader({
  children,
}: {
  children?: React.ReactNode;
}) {
  return (
    <SidebarInset>
      <AgentHeader
        name="Customer Insights Engine"

        status="active"
        integrations={[
          { name: "Slack", icon: <Slack /> },
          { name: "GitHub", icon: <GithubLight /> },
        ]}
        primaryAction={{ label: "Pause", icon: <PauseIcon className="size-4" />, onClick: () => {} }}
        onMenuClick={() => {}}
      />
      <div className="flex-1 p-6 md:p-8">{children}</div>
    </SidebarInset>
  );
}

const meta = {
  title: "Layout/ContentArea",
  component: ContentAreaWithBreadcrumbs,
  decorators: [
    (Story, context) => {
      const route = (context.parameters.route as string) ?? "/";
      return (
        <MemoryRouter initialEntries={[route]}>
          <SidebarProvider>
            <AppSidebar isAuthenticated={false} />
            <Story />
          </SidebarProvider>
        </MemoryRouter>
      );
    },
  ],
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof ContentAreaWithBreadcrumbs>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {};

export const WithContent: Story = {
  render: () => (
    <ContentAreaWithBreadcrumbs>
      <h1 className="text-2xl font-semibold">Page Title</h1>
      <p className="mt-2 text-muted-foreground">
        This is the main content area with the inset layout. The content sits
        inside a white rounded panel with a border, inset from the sidebar
        background.
      </p>
    </ContentAreaWithBreadcrumbs>
  ),
};

export const WithAgentHeader: Story = {
  name: "With Agent Header",
  render: () => (
    <ContentAreaWithAgentHeader>
      <h1 className="text-2xl font-semibold">Agent Dashboard</h1>
      <p className="mt-2 text-muted-foreground">
        This variant uses the AgentHeader instead of breadcrumbs, showing agent
        identity, status, and integrations.
      </p>
    </ContentAreaWithAgentHeader>
  ),
};
