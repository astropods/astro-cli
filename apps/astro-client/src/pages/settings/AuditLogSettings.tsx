import type { MetaFunction } from "react-router";
import { useAuth } from "@/lib/auth";
import { AuditLogView } from "@/components/settings/AuditLogView";

export const meta: MetaFunction = () => [
  { title: "Audit Log - Settings | Astro" },
];

export default function AuditLogSettings() {
  const { personalAccount } = useAuth();
  const accountName = personalAccount?.name ?? "";
  return (
    <AuditLogView
      account={accountName}
      subtitle="A record of actions taken on your account"
    />
  );
}
