import type { MetaFunction } from "react-router";
import { useAuth } from "@/lib/auth";
import { BillingView } from "@/components/settings/BillingView";

export const meta: MetaFunction = () => [{ title: "Billing - Settings | Astro" }];

export default function BillingSettings() {
  const { personalAccount } = useAuth();
  return <BillingView account={personalAccount?.name ?? ""} />;
}
