import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router-dom";

import { AppSidebar } from "@/components/AppSidebar";
import { SidebarProvider } from "@/components/ui/sidebar";
import type { User } from "@/lib/api";

const mockUser: User = {
  id: "usr_1",
  email: "jane@acme.com",
  first_name: "Jane",
  last_name: "Smith",
  email_verified: true,
  created_at: "2025-01-01T00:00:00Z",
  updated_at: "2025-01-01T00:00:00Z",
};

const mockUserWithAvatar: User = {
  ...mockUser,
  profile_picture_url: "https://i.pravatar.cc/150?u=jane",
};

const mockUserEmailOnly: User = {
  id: "usr_2",
  email: "developer@longcompanyname.io",
  email_verified: true,
  created_at: "2025-01-01T00:00:00Z",
  updated_at: "2025-01-01T00:00:00Z",
};

const meta = {
  title: "Components/Sidebar/AppSidebar",
  component: AppSidebar,
  decorators: [
    (Story, context) => {
      const route = (context.parameters.route as string) ?? "/";
      return (
        <MemoryRouter initialEntries={[route]}>
          <SidebarProvider>
            <Story />
          </SidebarProvider>
        </MemoryRouter>
      );
    },
  ],
  args: {},
  argTypes: {
    user: { control: false },
    isLoading: { control: "boolean" },
    isAuthenticated: { control: "boolean" },
    onSignIn: { table: { disable: true } },
    onSignOut: { table: { disable: true } },
    onTalkToAstro: { table: { disable: true } },
  },
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof AppSidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SignedOut: Story = {
  args: {
    isAuthenticated: false,
    isLoading: false,
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};

export const SignedInWithAvatar: Story = {
  args: {
    user: mockUserWithAvatar,
    isAuthenticated: true,
  },
};

export const SignedInWithInitials: Story = {
  args: {
    user: mockUser,
    isAuthenticated: true,
  },
};

export const SignedInEmailOnly: Story = {
  name: "Signed In (email-only user)",
  args: {
    user: mockUserEmailOnly,
    isAuthenticated: true,
  },
};

export const ActiveHireAgents: Story = {
  name: "Active State: Hire Agents",
  parameters: { route: "/hire" },
  args: {
    user: mockUserWithAvatar,
    isAuthenticated: true,
  },
};

export const ActiveOperator: Story = {
  name: "Active State: Operator",
  parameters: { route: "/operator" },
  args: {
    user: mockUserWithAvatar,
    isAuthenticated: true,
  },
};
