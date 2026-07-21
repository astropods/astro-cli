import { useParams, type MetaFunction } from 'react-router'
import { useAuth } from "@/lib/auth";
import { BillingView } from "@/components/settings/BillingView";

export const meta: MetaFunction = () => [{ title: "Billing - Organization Settings | Astro" }];

export default function OrgBillingSettings() {
  const { orgSlug = '' } = useParams()
  const { role } = useAuth()
  const canRequestIncrease = role === 'admin' || role === 'owner'
  return <BillingView account={orgSlug} canRequestIncrease={canRequestIncrease} />
}
