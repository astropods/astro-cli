import { OrgSwitcher } from "@/components/OrgSwitcher";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";

export function PageScopeSwitcher() {
  const { isAuthenticated } = useAuth();
  const { activeAccount, setActiveAccount } = useActiveAccount();

  if (!isAuthenticated) return null;

  return (
    <OrgSwitcher
      activeAccount={activeAccount}
      onChange={setActiveAccount}
    />
  );
}
