import { Outlet, useParams, Link } from 'react-router'
import { isOrgAdmin } from "@/lib/roles";
import { useEffect, useRef, useState } from 'react'
import { KeyRound, Database, ArrowLeft, Settings, Loader2, Users, ScrollText, FlaskConical } from 'lucide-react'
import { CreditCardIcon, ChartBarIcon } from '@heroicons/react/24/outline'
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from '@/components/ui/sidebar-layout'
import { useAuth } from '@/lib/auth'

function OrgSettingsContent() {
  const { orgSlug = '' } = useParams()
  const { accounts, organizationId, role, switchOrg } = useAuth()
  const org = accounts.find(a => a.name === orgSlug)
  const needsSwitch = !!org?.organization_id && org.organization_id !== organizationId
  const [switchFailed, setSwitchFailed] = useState(false)

  // Track whether we've ever resolved a valid org in this component instance.
  // If org disappears after being valid (e.g. during a rename), render nothing
  // instead of flashing the 403 page during the transition.
  const hasResolvedOrg = useRef(false)
  if (org) hasResolvedOrg.current = true

  useEffect(() => {
    if (!needsSwitch || !org?.organization_id) return
    setSwitchFailed(false)
    switchOrg(org.organization_id).catch(() => {
      // Switch failed (e.g. new org not yet ready in WorkOS) — render anyway
      setSwitchFailed(true)
    })
  }, [needsSwitch, org?.organization_id, switchOrg])

  if (!org) {
    // Transitional state (e.g. rename in progress)
    if (hasResolvedOrg.current) {
      return (
        <div className="flex items-center justify-center flex-1">
          <Loader2 size={20} className="animate-spin text-muted-foreground" />
        </div>
      )
    }

    return (
      <div className="flex items-center justify-center flex-1">
        <div className="text-center">
          <h1 className="text-7xl font-extrabold mb-2">403</h1>
          <p className="text-xl font-semibold mb-2">Access denied</p>
          <p className="text-stone-600 text-sm mb-6">
            You don't have permission to view this organization's settings.
          </p>
          <Link
            to="/settings/organizations"
            className="inline-block px-4 py-2 bg-stone-800 text-white border border-stone-800 text-sm no-underline"
          >
            Back to organizations
          </Link>
        </div>
      </div>
    )
  }

  if (needsSwitch && !switchFailed) return null

  if (switchFailed) {
    return (
      <div className="flex items-center justify-center flex-1">
        <div className="text-center">
          <p className="text-xl font-semibold mb-2">Something went wrong</p>
          <p className="text-stone-600 text-sm mb-6">
            Failed to load this organization's context. This can happen if the
            organization was just created. Try refreshing the page.
          </p>
          <div className="flex items-center justify-center gap-3">
            <button
              onClick={() => window.location.reload()}
              className="inline-block px-4 py-2 bg-stone-800 text-white border border-stone-800 text-sm no-underline cursor-pointer"
            >
              Refresh
            </button>
            <Link
              to="/settings/organizations"
              className="inline-block px-4 py-2 border border-stone-300 text-stone-800 text-sm no-underline"
            >
              Back to organizations
            </Link>
          </div>
        </div>
      </div>
    )
  }

  const displayName = org.display_name ?? orgSlug
  const isAdmin = isOrgAdmin(role)

  return (
    <div className="flex-1 overflow-y-auto bg-background">
      <div className="@container w-full px-4 pb-6 pt-8 md:px-6 md:pb-8 md:pt-10 max-w-[1120px] mx-auto">
        <SidebarLayout>
        <div className="flex w-full min-w-0 flex-col md:w-48 md:shrink-0">
          <div className="mb-4">
            <Link
              to="/settings/account"
              className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors mb-2"
            >
              <ArrowLeft className="size-3" />
              Settings
            </Link>
            <h1 className="flex max-w-full items-start gap-2 text-heading-2 text-foreground">
              <span
                className="min-w-0 max-w-full hyphens-auto [overflow-wrap:anywhere]"
                title={displayName}
              >
                {displayName}
              </span>
            </h1>
          </div>
          <SidebarNav label="Org settings" className="md:w-48">
            <SidebarNavItem to={`/settings/org/${orgSlug}/general`}>
              <span className="flex items-center gap-2">
                <Settings className="size-3.5" />
                General
              </span>
            </SidebarNavItem>
            <SidebarNavItem to={`/settings/org/${orgSlug}/members`}>
              <span className="flex items-center gap-2">
                <Users className="size-3.5" />
                Members
              </span>
            </SidebarNavItem>
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/usage`}>
                <span className="flex items-center gap-2">
                  <ChartBarIcon className="size-3.5" />
                  Usage
                </span>
              </SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/billing`}>
                <span className="flex items-center gap-2">
                  <CreditCardIcon className="size-3.5" />
                  Billing
                </span>
              </SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/secrets`}>
                <span className="flex items-center gap-2">
                  <KeyRound className="size-3.5" />
                  Secrets & Variables
                </span>
              </SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/api-keys`}>
                <span className="flex items-center gap-2">
                  <Database className="size-3.5" />
                  Data Sources
                </span>
              </SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/apps`}>
                <span className="flex items-center gap-2">
                  <KeyRound className="size-3.5" />
                  Apps
                </span>
              </SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/audit-log`}>
                <span className="flex items-center gap-2">
                  <ScrollText className="size-3.5" />
                  Audit Log
                </span>
              </SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/experiments`}>
                <span className="flex items-center gap-2">
                  <FlaskConical className="size-3.5" />
                  Experiments
                </span>
              </SidebarNavItem>
            )}
          </SidebarNav>
        </div>
        <SidebarBody>
          <Outlet />
        </SidebarBody>
        </SidebarLayout>
      </div>
    </div>
  )
}

export default function OrgSettingsLayout() {
  return <OrgSettingsContent />
}
