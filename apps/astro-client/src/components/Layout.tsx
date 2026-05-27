import { Outlet } from "react-router";
import { AppHeader } from "./AppHeader";
import { NavigationProgressBar } from "./NavigationProgressBar";
import { ActiveAccountProvider } from "@/hooks/use-active-account";

export default function Layout() {
  return (
    <ActiveAccountProvider>
      <NavigationProgressBar />
      <div className="flex min-h-dvh flex-col bg-background">
        <AppHeader />
        <Outlet />
      </div>
    </ActiveAccountProvider>
  );
}
