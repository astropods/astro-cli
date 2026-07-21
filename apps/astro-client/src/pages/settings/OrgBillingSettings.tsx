import { useParams, Link, type MetaFunction } from 'react-router'
import { useAuth } from "@/lib/auth";
import { BillingView } from "@/components/settings/BillingView";

export const meta: MetaFunction = () => [{ title: "Billing - Organization Settings | Astro" }];

export default function OrgBillingSettings() {
  const { orgSlug = '' } = useParams()
  const { role } = useAuth()
  const isAdmin = role === 'admin' || role === 'owner'

  // Billing is restricted to org admins/owners. Guard direct navigation
  // (the nav item is hidden for members, but the URL is still reachable).
  if (!isAdmin) {
    return (
      <div className="flex items-center justify-center flex-1">
        <div className="text-center">
          <h1 className="text-7xl font-extrabold mb-2">403</h1>
          <p className="text-xl font-semibold mb-2">Access denied</p>
          <p className="text-muted-foreground text-body-sm mb-6">
            Only organization admins and owners can view billing.
          </p>
          <Link
            to={`/settings/org/${orgSlug}/general`}
            className="inline-block px-4 py-2 bg-primary text-primary-foreground text-body-sm no-underline"
          >
            Back to organization settings
          </Link>
        </div>
      </div>
    )
  }

  return <BillingView account={orgSlug} canRequestIncrease={isAdmin} />
}
