import { useParams, type MetaFunction } from "react-router";
import { AuditLogView } from "@/components/settings/AuditLogView";

export const meta: MetaFunction = () => [
  { title: "Audit Log - Organization Settings | Astro" },
];

export default function OrgAuditLogSettings() {
  const { orgSlug = "" } = useParams();
  return (
    <AuditLogView
      account={orgSlug}
      subtitle="A record of actions taken in this organization"
    />
  );
}
