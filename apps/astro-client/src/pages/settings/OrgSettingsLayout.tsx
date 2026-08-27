import { Outlet, useParams, Link } from 'react-router'
import { isOrgAdmin } from "@/lib/roles";
import { useEffect, useRef, useState } from 'react'
import { Loader2 } from 'lucide-react'
import {
  SidebarLayout,
  SidebarNavItem,
  SidebarNavGroup,
  SidebarNavDivider,
  SidebarNavPlaceholder,
  SidebarBody,
} from '@/components/ui/sidebar-layout'
import { SettingsSidebar } from '@/components/settings/SettingsSidebar'
import { Button } from '@/components/ui/button'
import { Tag } from '@/components/Tag'
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
          <p className="text-muted-foreground text-sm mb-6">
            You don't have permission to view this organization's settings.
          </p>
          <Button asChild>
            <Link to="/settings/organizations">Back to organizations</Link>
          </Button>
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
          <p className="text-muted-foreground text-sm mb-6">
            Failed to load this organization's context. This can happen if the
            organization was just created. Try refreshing the page.
          </p>
          <div className="flex items-center justify-center gap-3">
            <Button onClick={() => window.location.reload()}>Refresh</Button>
            <Button variant="outline" asChild>
              <Link to="/settings/organizations">Back to organizations</Link>
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const isAdmin = isOrgAdmin(role)

  return (
    <div className="flex-1 overflow-y-auto bg-background">
      <div className="@container w-full px-4 pb-6 pt-8 md:px-6 md:pb-8 md:pt-10 max-w-[1120px] mx-auto">
        <SidebarLayout>
        <SettingsSidebar account={orgSlug}>
          <SidebarNavGroup label="Manage">
            <SidebarNavItem to={`/settings/org/${orgSlug}/general`}>Account</SidebarNavItem>
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/billing`}>Billing</SidebarNavItem>
            )}
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/usage`}>Usage</SidebarNavItem>
            )}
          </SidebarNavGroup>
          <SidebarNavGroup label="Access">
            <SidebarNavItem to={`/settings/org/${orgSlug}/members`}>Members</SidebarNavItem>
            {isAdmin && (
              <SidebarNavItem to={`/settings/org/${orgSlug}/audit-log`}>Audit Log</SidebarNavItem>
            )}
          </SidebarNavGroup>
          {isAdmin && (
            <SidebarNavGroup label="Integrations">
              <SidebarNavItem to={`/settings/org/${orgSlug}/secrets`}>Variables &amp; Secrets</SidebarNavItem>
              <SidebarNavPlaceholder note="Coming soon">Connectors</SidebarNavPlaceholder>
              <SidebarNavItem to={`/settings/org/${orgSlug}/api-keys`}>Data Sources</SidebarNavItem>
              <SidebarNavItem to={`/settings/org/${orgSlug}/apps`}>OAuth Apps</SidebarNavItem>
            </SidebarNavGroup>
          )}
          {isAdmin && (
            <>
              <SidebarNavDivider />
              <SidebarNavItem
                to={`/settings/org/${orgSlug}/experiments`}
                className="flex items-center gap-2"
              >
                Experiments
                <Tag>Beta</Tag>
              </SidebarNavItem>
            </>
          )}
        </SettingsSidebar>
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
