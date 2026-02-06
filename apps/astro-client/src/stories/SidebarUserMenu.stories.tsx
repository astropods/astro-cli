import type { Meta, StoryObj } from "@storybook/react-vite";

import { SidebarUserMenu } from "@/components/sidebar/SidebarUserMenu";
import { SidebarProvider, Sidebar, SidebarFooter } from "@/components/ui/sidebar";
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
  title: "Components/Sidebar/UserMenu",
  component: SidebarUserMenu,
  decorators: [
    (Story) => (
      <SidebarProvider className="min-h-0">
        <Sidebar collapsible="none" className="h-auto">
          <SidebarFooter className="py-3">
            <Story />
          </SidebarFooter>
        </Sidebar>
      </SidebarProvider>
    ),
  ],
  argTypes: {
    user: { control: false },
    isLoading: { control: "boolean" },
    isAuthenticated: { control: "boolean" },
    onSignIn: { table: { disable: true } },
    onSignOut: { table: { disable: true } },
  },
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof SidebarUserMenu>;

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

export const WithAvatar: Story = {
  args: {
    user: mockUserWithAvatar,
    isAuthenticated: true,
  },
};

export const WithInitials: Story = {
  args: {
    user: mockUser,
    isAuthenticated: true,
  },
};

export const EmailOnly: Story = {
  name: "Email-only user",
  args: {
    user: mockUserEmailOnly,
    isAuthenticated: true,
  },
};
