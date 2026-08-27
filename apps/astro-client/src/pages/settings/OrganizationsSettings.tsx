import { useState } from 'react'
import { Link, type MetaFunction } from 'react-router'
import { PlusIcon } from '@heroicons/react/24/outline'
import { Button } from '@/components/ui/button'
import { Tag, type TagColor } from '@/components/Tag'
import { UserAvatar } from '@/components/UserAvatar'
import { SectionHeader } from '@/components/settings/SettingsShared'
import { LeaveOrganizationDialog } from '@/components/settings/LeaveOrganizationDialog'
import { useAuth } from '@/lib/auth'

export const meta: MetaFunction = () => [{ title: "Organizations - Settings | Astro" }];

const ORG_ROLE_TAG: Record<string, { label: string; color: TagColor }> = {
  owner:  { label: 'Owner',  color: 'yellow'  },
  admin:  { label: 'Admin',  color: 'blue'    },
  member: { label: 'Member', color: 'default' },
}

export default function OrganizationsSettings() {
  const { accounts } = useAuth()
  const orgs = accounts.filter(a => a.type === 'organization')
  const [leaving, setLeaving] = useState<string | null>(null)

  return (
    <>
      <SectionHeader
        title="Organizations"
        subtitle="Manage organizations you have created or joined"
        action={
          <Button size="sm" asChild>
            <Link to="/organization/new">
              <PlusIcon className="size-3.5" />
              Create organization
            </Link>
          </Button>
        }
      />

      {orgs.length === 0 ? (
        <p className="text-[13px] text-muted-foreground py-4">You're not a member of any organizations yet.</p>
      ) : (
        <div className="space-y-3">
          {orgs.map(org => {
            const roleTag = org.role ? ORG_ROLE_TAG[org.role.toLowerCase()] : null
            return (
              <div
                key={org.name}
                className="flex items-center gap-4 px-4 py-3 rounded-lg border border-border"
              >
                <UserAvatar
                  handle={org.name}
                  name={org.display_name || org.name}
                  avatarUrl={org.avatar_url}
                  className="size-9 shrink-0 ring-1 ring-border"
                />

                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground">{org.display_name || org.name}</p>
                  {roleTag && (
                    <div className="mt-1">
                      <Tag color={roleTag.color}>{roleTag.label}</Tag>
                    </div>
                  )}
                </div>

                <Button
                  variant="outline"
                  size="sm"
                  className="shrink-0 text-error"
                  onClick={() => setLeaving(org.name)}
                >
                  Leave
                </Button>
              </div>
            )
          })}
        </div>
      )}

      {leaving && (
        <LeaveOrganizationDialog
          orgSlug={leaving}
          open
          onOpenChange={open => !open && setLeaving(null)}
        />
      )}
    </>
  )
}
