import { Outlet } from "react-router";
import { AppHeader } from "./AppHeader";
import { BillingStatusBanner } from "./BillingStatusBanner";
import { NavigationProgressBar } from "./NavigationProgressBar";
import { ActiveAccountProvider } from "@/hooks/use-active-account";

export default function Layout() {
  return (
    <ActiveAccountProvider>
      <NavigationProgressBar />
      <div className="flex h-dvh max-h-dvh flex-col overflow-hidden bg-background">
        <AppHeader />
        <main className="flex min-h-0 flex-1 flex-col overflow-y-auto">
          <BillingStatusBanner className="px-6 pt-4" />
          <Outlet />
        </main>
      </div>
    </ActiveAccountProvider>
  );
}
