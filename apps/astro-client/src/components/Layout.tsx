import { Outlet } from "react-router";
import { AppHeader } from "./AppHeader";
import { ActiveAccountProvider } from "@/hooks/use-active-account";

export default function Layout() {
  return (
    <ActiveAccountProvider>
      <div className="flex min-h-dvh flex-col bg-muted">
        <AppHeader />
        <Outlet />
      </div>
    </ActiveAccountProvider>
  );
}
