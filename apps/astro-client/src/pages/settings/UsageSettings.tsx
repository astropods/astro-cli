import type { MetaFunction } from "react-router";
import { useAuth } from "@/lib/auth";
import { UsageView } from "@/components/settings/UsageView";

export const meta: MetaFunction = () => [{ title: "Usage - Settings | Astro" }];

export default function UsageSettings() {
  const { personalAccount, isLoading } = useAuth();
  // Wait for personalAccount to resolve; an empty account would disable the
  // query and read as "billing isn't enabled" instead of loading.
  if (isLoading) return null;
  return <UsageView account={personalAccount?.name ?? ""} />;
}
