import { Outlet, useParams, Link } from 'react-router'
import { useEffect, useState } from 'react'
import { KeyRound, ArrowLeft, Loader2 } from 'lucide-react'
import { ProtectedRoute } from '@/components/ProtectedRoute'
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from '@/components/ui/sidebar-layout'
import { useAuth } from '@/lib/auth'

function OrgSettingsContent() {
  const { orgSlug = '' } = useParams()
  const { accounts, organizationId, switchOrg } = useAuth()
  const org = accounts.find(a => a.name === orgSlug)
  const displayName = org?.display_name ?? orgSlug
  const [switching, setSwitching] = useState(false)

  useEffect(() => {
    if (!org?.organization_id || org.organization_id === organizationId) return
    let mounted = true
    setSwitching(true)
    switchOrg(org.organization_id).finally(() => { if (mounted) setSwitching(false) })
    return () => { mounted = false }
  }, [org?.organization_id, organizationId, switchOrg])

  if (switching) {
    return (
      <div className="flex items-center gap-2 py-8 px-6 text-[13px] text-muted-foreground">
        <Loader2 size={14} className="animate-spin" />
        Switching organization context...
      </div>
    )
  }

  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-4 pb-6 pt-8 md:px-6 md:pb-8 md:pt-10 max-w-[1120px] mx-auto">
      <div className="mb-6">
        <Link
          to="/settings/account"
          className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors mb-3"
        >
          <ArrowLeft className="size-3" />
          Settings
        </Link>
        <h1 className="text-heading-2 text-foreground">{displayName}</h1>
      </div>

      <SidebarLayout>
        <SidebarNav label="Org settings" className="md:w-48">
          <SidebarNavItem to={`/settings/org/${orgSlug}/secrets`}>
            <span className="flex items-center gap-2">
              <KeyRound className="size-3.5" />
              Secrets & Variables
            </span>
          </SidebarNavItem>
        </SidebarNav>
        <SidebarBody>
          <Outlet />
        </SidebarBody>
      </SidebarLayout>
    </div>
  )
}

export default function OrgSettingsLayout() {
  return (
    <ProtectedRoute>
      <OrgSettingsContent />
    </ProtectedRoute>
  )
}
