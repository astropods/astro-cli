import type { MetaFunction } from "react-router";
import { useAuth } from "@/lib/auth";
import { UsageView } from "@/components/settings/UsageView";

export const meta: MetaFunction = () => [{ title: "Usage - Settings | Astro" }];

export default function UsageSettings() {
  const { personalAccount } = useAuth();
  return <UsageView account={personalAccount?.name ?? ""} />;
}
