import { useParams, type MetaFunction } from 'react-router'
import { useAuth } from "@/lib/auth";
import { UsageView } from "@/components/settings/UsageView";

export const meta: MetaFunction = () => [{ title: "Usage - Organization Settings | Astro" }];

export default function OrgUsageSettings() {
  const { orgSlug = '' } = useParams()
  const { role } = useAuth()
  const canRequestIncrease = role === 'admin' || role === 'owner'
  return <UsageView account={orgSlug} canRequestIncrease={canRequestIncrease} />
}
